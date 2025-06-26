package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/alecthomas/kong"
	zabbixsender "github.com/hnakamur/go-zabbixsender"
)

var cli struct {
	Debug bool `help:"Enable debug mode."`

	Send SendCmd `cmd:"" help:"Send a metric to a Zabbix server."`
}

type SendCmd struct {
	Host  string    `required:"" help:"Hostname for the metric"`
	Key   string    `required:"" help:"metric item key"`
	Value string    `required:"" help:"metric value"`
	Time  time.Time `format:"2006-01-02T15:04:05.999999999" help:"time for the metric in yyyy-mm-ddTHH:MM:SS(.sssssssss)? format"`

	Server  string        `required:"" help:"Zabbix server address in host:port format."`
	Timeout time.Duration `default:"5s" help:"send timeout"`
}

func (c *SendCmd) Run(ctx context.Context) error {
	sender := zabbixsender.Sender{ServerAddress: c.Server, Timeout: c.Timeout}
	var clock, ns int64
	if !c.Time.IsZero() {
		clock = c.Time.Unix()
		ns = c.Time.UnixNano() % int64(time.Second)
	}
	resp, err := sender.Send([]zabbixsender.TrapperData{
		{
			Host:  c.Host,
			Key:   c.Key,
			Value: c.Value,
			Clock: clock,
			Ns:    ns,
		},
	})
	if err != nil {
		return err
	}
	slog.Info("sent metrics", "response", resp)
	return nil
}

func main() {
	slogLevel := new(slog.LevelVar)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel}))
	slog.SetDefault(logger)

	ctx := kong.Parse(&cli)
	if cli.Debug {
		slogLevel.Set(slog.LevelDebug)
	}
	// kong.BindTo is needed to bind a context.Context value.
	// See https://github.com/alecthomas/kong/issues/48
	ctx.BindTo(context.Background(), (*context.Context)(nil))
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
