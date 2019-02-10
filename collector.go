package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/barnybug/miflora"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricPrefix = "flowercare_"

	// Conversion factor from µS/cm to S/m
	factorConductivity = 0.0001
)

type sensorData struct {
	Time     time.Time
	Firmware miflora.Firmware
	Sensors  miflora.Sensors
}

type flowercareCollector struct {
	MacAddress    string
	Device        string
	CacheDuration time.Duration

	cache               sensorData
	upMetric            prometheus.Gauge
	scrapeErrorsMetric  prometheus.Counter
	scrapeTimestampDesc *prometheus.Desc
	infoDesc            *prometheus.Desc
	batteryDesc         *prometheus.Desc
	conductivityDesc    *prometheus.Desc
	lightDesc           *prometheus.Desc
	moistureDesc        *prometheus.Desc
	temperatureDesc     *prometheus.Desc
}

func newCollector(macAddress, device string, cacheDuration time.Duration) *flowercareCollector {
	constLabels := prometheus.Labels{
		"macaddress": strings.ToLower(macAddress),
	}

	return &flowercareCollector{
		MacAddress:    macAddress,
		Device:        device,
		CacheDuration: cacheDuration,

		upMetric: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        metricPrefix + "up",
			Help:        "Shows if data could be successfully retrieved by the collector.",
			ConstLabels: constLabels,
		}),
		scrapeErrorsMetric: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        metricPrefix + "scrape_errors_total",
			Help:        "Counts the number of scrape errors by this collector.",
			ConstLabels: constLabels,
		}),
		scrapeTimestampDesc: prometheus.NewDesc(
			metricPrefix+"scrape_timestamp",
			"Contains the timestamp when the last communication with the Bluetooth device happened.",
			nil, constLabels),
		infoDesc: prometheus.NewDesc(
			metricPrefix+"info",
			"Contains information about the Flower Care device.",
			[]string{"version"}, constLabels),
		batteryDesc: prometheus.NewDesc(
			metricPrefix+"battery_percent",
			"Battery level in percent.",
			nil, constLabels),
		conductivityDesc: prometheus.NewDesc(
			metricPrefix+"conductivity_sm",
			"Soil conductivity in Siemens/meter.",
			nil, constLabels),
		lightDesc: prometheus.NewDesc(
			metricPrefix+"brightness_lux",
			"Ambient lighting in lux.",
			nil, constLabels),
		moistureDesc: prometheus.NewDesc(
			metricPrefix+"moisture_percent",
			"Soil relative moisture in percent.",
			nil, constLabels),
		temperatureDesc: prometheus.NewDesc(
			metricPrefix+"temperature_celsius",
			"Ambient temperature in celsius.",
			nil, constLabels),
	}
}

func (c *flowercareCollector) Describe(ch chan<- *prometheus.Desc) {
	c.upMetric.Describe(ch)
	c.scrapeErrorsMetric.Describe(ch)

	ch <- c.scrapeTimestampDesc
	ch <- c.infoDesc
	ch <- c.batteryDesc
	ch <- c.conductivityDesc
	ch <- c.lightDesc
	ch <- c.moistureDesc
	ch <- c.temperatureDesc
}

func (c *flowercareCollector) Collect(ch chan<- prometheus.Metric) {
	if time.Since(c.cache.Time) > c.CacheDuration {
		data, err := c.readData()
		if err != nil {
			log.Printf("Error during scrape: %s", err)

			c.scrapeErrorsMetric.Inc()
			c.upMetric.Set(0)
		} else {
			c.upMetric.Set(1)
			c.cache = *data
		}
	}

	c.upMetric.Collect(ch)
	c.scrapeErrorsMetric.Collect(ch)

	if time.Since(c.cache.Time) < c.CacheDuration {
		if err := c.collectData(ch, c.cache); err != nil {
			log.Printf("Error collecting metrics: %s", err)
		}
	}
}

func (c *flowercareCollector) collectData(ch chan<- prometheus.Metric, data sensorData) error {
	if err := sendMetric(ch, c.scrapeTimestampDesc, float64(data.Time.Unix())); err != nil {
		return err
	}

	if err := sendMetric(ch, c.infoDesc, 1, data.Firmware.Version); err != nil {
		return err
	}

	for _, metric := range []struct {
		Desc  *prometheus.Desc
		Value float64
	}{
		{
			Desc:  c.batteryDesc,
			Value: float64(data.Firmware.Battery),
		},
		{
			Desc:  c.conductivityDesc,
			Value: float64(data.Sensors.Conductivity) * factorConductivity,
		},
		{
			Desc:  c.lightDesc,
			Value: float64(data.Sensors.Light),
		},
		{
			Desc:  c.moistureDesc,
			Value: float64(data.Sensors.Moisture),
		},
		{
			Desc:  c.temperatureDesc,
			Value: data.Sensors.Temperature,
		},
	} {
		if err := sendMetric(ch, metric.Desc, metric.Value); err != nil {
			return err
		}
	}

	return nil
}

func (c *flowercareCollector) readData() (*sensorData, error) {
	f := miflora.NewMiflora(c.MacAddress, c.Device)

	firmware, err := f.ReadFirmware()
	if err != nil {
		return nil, fmt.Errorf("can not read firmware: %s", err)
	}

	sensors, err := f.ReadSensors()
	if err != nil {
		return nil, fmt.Errorf("can not read sensors: %s", err)
	}

	return &sensorData{
		Time:     time.Now(),
		Firmware: firmware,
		Sensors:  sensors,
	}, nil
}

func sendMetric(ch chan<- prometheus.Metric, desc *prometheus.Desc, value float64, labels ...string) error {
	m, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, value, labels...)
	if err != nil {
		return fmt.Errorf("can not create metric %q: %s", desc, err)
	}
	ch <- m

	return nil
}
