package main

// Use the following command to build a static binary.
//
// go build -trimpath -tags netgo,osusergo

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/alecthomas/kong"
	zabbixsender "github.com/hnakamur/go-zabbixsender"
)

var cli struct {
	Debug bool `help:"Enable debug mode."`

	Send    SendCmd    `cmd:"" help:"Send a metric to a Zabbix server."`
	Run     RunCmd     `cmd:"" help:"Run a command and send metrics (start time before running, elapsed time, and exit code after running)."`
	Version VersionCmd `cmd:"" help:"Show version and exit."`
}

type SendCmd struct {
	Host  string    `group:"metric" required:"" help:"Hostname for the metric"`
	Key   string    `group:"metric" required:"" help:"metric item key"`
	Value string    `group:"metric" required:"" help:"metric value"`
	Time  time.Time `group:"metric" format:"2006-01-02T15:04:05.999999999" help:"time for the metric in yyyy-mm-ddTHH:MM:SS(.sssssssss)? format"`

	Server  string        `group:"zabbix_server" required:"" help:"Zabbix server address in host:port format."`
	Timeout time.Duration `group:"zabbix_server" default:"5s" help:"send timeout"`
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

type RunCmd struct {
	Host              string `group:"metric" required:"" help:"Hostname for the metric"`
	Prefix            string `group:"metric" required:"" help:"metric item key prefix"`
	StartTimeSuffix   string `group:"metric" default:"_start_time" help:"metric item key suffix for start time"`
	ElapsedTimeSuffix string `group:"metric" default:"_elapsed_time" help:"metric item key suffix for elapsed time"`
	ExitCodeSuffix    string `group:"metric" default:"_exit_code" help:"metric item key suffix for exit code"`

	Server  string        `group:"zabbix_server" required:"" help:"Zabbix server address in host:port format."`
	Timeout time.Duration `group:"zabbix_server" default:"5s" help:"send timeout"`

	Command string   `group:"exec" arg:"" help:"path to command to be executed"`
	Args    []string `group:"exec" arg:"" optional:"" help:"arguments for the command to be executed"`
}

func (c *RunCmd) Run(ctx context.Context) error {
	sender := zabbixsender.Sender{ServerAddress: c.Server, Timeout: c.Timeout}
	startTime := time.Now()

	{
		resp, err := sender.Send([]zabbixsender.TrapperData{
			{
				Host:  c.Host,
				Key:   c.Prefix + c.StartTimeSuffix,
				Value: formatTimeToSecondsFromEpoch(startTime),
			},
		})
		if err != nil {
			slog.Error("failed to sent pre-run metric",
				"keyPrefix", c.Prefix, "err", err)
		} else {
			slog.Info("sent pre-run metric", "response", resp)
		}
	}

	exitCode, cmdErr := runCommand(ctx, c.Command, c.Args...)
	if cmdErr != nil {
		slog.Error("command failed", "err", cmdErr)
	}

	elapsed := time.Since(startTime)
	{
		resp, err := sender.Send([]zabbixsender.TrapperData{
			{
				Host:  c.Host,
				Key:   c.Prefix + c.ExitCodeSuffix,
				Value: strconv.Itoa(exitCode),
			},
			{
				Host:  c.Host,
				Key:   c.Prefix + c.ElapsedTimeSuffix,
				Value: formatElapsedTimeInSeconds(elapsed),
			},
		})
		if err != nil {
			slog.Error("failed to sent post-run metric",
				"keyPrefix", c.Prefix, "err", err)
		} else {
			slog.Info("sent post-run metrics", "response", resp)
		}
	}

	return cmdErr
}

func formatTimeToSecondsFromEpoch(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
}

func formatElapsedTimeInSeconds(d time.Duration) string {
	return fmt.Sprintf("%g", float64(d)/float64(time.Second))
}

func runCommand(ctx context.Context, command string, arg ...string) (int, error) {
	cmd := exec.CommandContext(ctx, command, arg...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	var exitCode int
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(kong.ExitCoder); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	return exitCode, err
}

type VersionCmd struct{}

func (c *VersionCmd) Run(ctx context.Context) error {
	fmt.Println(Version())
	return nil
}

func Version() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return "(devel)"
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
