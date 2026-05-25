package proxy

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"
)

// RunDaemon is the worker entry point invoked by `aitoll proxy daemon`. It
// loads the cached singbox.json, boots a *box.Box, and blocks until SIGTERM
// or SIGINT. All sing-box logs go to whatever stderr is wired to (the parent
// redirects it to proxy.log when forking).
func RunDaemon() error {
	cfgPath, err := SingboxConfigPath()
	if err != nil {
		return err
	}
	configBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("read singbox config %s: %w", cfgPath, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = include.Context(ctx)

	options, err := json.UnmarshalExtendedContext[option.Options](ctx, configBytes)
	if err != nil {
		return fmt.Errorf("parse singbox config: %w", err)
	}

	instance, err := box.New(box.Options{
		Context: ctx,
		Options: options,
	})
	if err != nil {
		return fmt.Errorf("create box: %w", err)
	}

	if err := instance.Start(); err != nil {
		_ = instance.Close()
		return fmt.Errorf("start box: %w", err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	return instance.Close()
}
