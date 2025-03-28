package main

import (
	"flag"
	"log"
	"time"

	zabbix "github.com/hnakamur/go-zabbixsender"
)

func main() {
	serverAddress := flag.String("server", "", "Zabbix server address")
	timeout := flag.Duration("timeout", 5*time.Second, "send timeout")
	hostname := flag.String("hostname", "", "hostname for the metric to send")
	key := flag.String("key", "", "metric key")
	value := flag.String("value", "", "metric, value")
	metricTimeStr := flag.String("time", "", "time for the metric in yyyy-mm-ddTHH:MM:SS(.sssssssss)? format")

	flag.Parse()

	var metricTime time.Time
	if *metricTimeStr != "" {
		var err error
		metricTime, err = time.ParseInLocation("2006-01-02T03:04:05.999999999", *metricTimeStr, time.Local)
		if err != nil {
			log.Fatal(err)
		}
	}
	if err := run(*serverAddress, *timeout, *hostname, *key, *value, metricTime); err != nil {
		log.Fatal(err)
	}
}

func run(serverAddress string, timeout time.Duration, hostname, key, value string, metricTime time.Time) error {
	sender := zabbix.Sender{ServerAddress: serverAddress, Timeout: timeout}
	var clock, ns int64
	if !metricTime.IsZero() {
		clock = metricTime.Unix()
		ns = metricTime.UnixNano() % int64(time.Second)
	}
	resp, err := sender.Send([]zabbix.TrapperData{
		{
			Host:  hostname,
			Key:   key,
			Value: value,
			Clock: clock,
			Ns:    ns,
		},
	})
	if err != nil {
		return err
	}
	log.Printf("resp=%+v", resp)
	return nil
}
