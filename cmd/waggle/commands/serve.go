package commands

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/lucientong/waggle/pkg/web"
)

// Serve implements the `waggle serve` command, starting the web visualization panel.
func Serve(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "TCP address to listen on (e.g., :8080)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := web.DefaultConfig()
	cfg.Addr = *addr

	srv := web.NewServer(cfg, nil, nil)

	// Start the server in a goroutine; shut it down when the context is cancelled.
	serverErr := make(chan error, 1)
	go func() {
		err := srv.Start()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	slog.Info("waggle web panel started", "url", fmt.Sprintf("http://localhost%s", *addr))
	fmt.Printf("Waggle visualization panel: http://localhost%s\n", *addr)
	fmt.Println("Press Ctrl+C to stop.")

	select {
	case <-ctx.Done():
		slog.Info("shutting down web server")
		return srv.Shutdown()
	case err := <-serverErr:
		return err
	}
}

// Version implements the `waggle version` command.
func Version() error {
	fmt.Println("waggle version 0.1.0 (Phase 7 — CLI)")
	return nil
}
