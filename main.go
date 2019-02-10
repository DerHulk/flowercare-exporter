package main

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/pflag"
)

type config struct {
	ListenAddr    string
	MacAddress    string
	Device        string
	CacheDuration time.Duration
}

func parseConfig() (config, error) {
	result := config{
		ListenAddr:    ":9294",
		Device:        "hci0",
		CacheDuration: 2 * time.Minute,
	}

	pflag.StringVarP(&result.ListenAddr, "addr", "a", result.ListenAddr, "Address to listen on for connections.")
	pflag.StringVarP(&result.MacAddress, "device", "b", result.MacAddress, "MAC-Address of Flower Care device.")
	pflag.StringVarP(&result.Device, "adapter", "i", result.Device, "Bluetooth device to use for communication.")
	pflag.DurationVarP(&result.CacheDuration, "cache-duration", "c", result.CacheDuration, "Interval during which the results from the Bluetooth device are cached.")
	pflag.Parse()

	if len(result.MacAddress) == 0 {
		return result, errors.New("need to provide a device address")
	}

	if len(result.Device) == 0 {
		return result, errors.New("need to provide a bluetooth device")
	}

	return result, nil
}

func main() {
	config, err := parseConfig()
	if err != nil {
		log.Fatalf("Error in configuration: %s", err)
	}

	log.Printf("Looking for %s via %s", config.MacAddress, config.Device)
	reader := dataReader(config.MacAddress, config.Device)

	collector := newCollector(reader, config.CacheDuration, config.MacAddress)
	if err := prometheus.Register(collector); err != nil {
		log.Fatalf("Failed to register collector: %s", err)
	}

	http.Handle("/metrics", prometheus.UninstrumentedHandler())
	http.Handle("/", http.RedirectHandler("/metrics", http.StatusFound))

	log.Printf("Listen on %s...", config.ListenAddr)
	log.Fatal(http.ListenAndServe(config.ListenAddr, nil))
}
