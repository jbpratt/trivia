package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jbpratt/bots/internal/triviabot"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	serverURL := "wss://chat.strims.gg/ws"
	dbPath := flag.String("db", "/tmp/trivia.db", "path to sqlite database")
	dev := flag.Bool("dev", false, "use chat2")
	lvl := zap.LevelFlag("v", zapcore.InfoLevel, "set the log level")
	leaderboardPage := flag.String("html", "/tmp/leaderboard/index.html", "path to output generated leaderboard page")
	leaderboardIngress := flag.String("ingress", "https://leaderboard.jbpratt.xyz", "leaderboard ingress URL")

	flag.Parse()

	if *dev {
		serverURL = "wss://chat2.strims.gg/ws"
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	atom := zap.NewAtomicLevelAt(*lvl)
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		atom,
	))

	defer func() {
		if err := logger.Sync(); err != nil {
			log.Fatal(err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-c
		logger.Sugar().Infow("received signal, shutting down", "signal", s)
		cancel()
	}()

	url, jwt := os.Getenv("STRIMS_CHAT_WSS_URL"), os.Getenv("STRIMS_CHAT_TOKEN")
	if url == "" {
		url = serverURL
	}
	if jwt == "" {
		logger.Fatal("must provide $STRIMS_CHAT_TOKEN")
	}

	triviabot, err := triviabot.New(ctx, logger.Sugar(), url, jwt, *dbPath, *leaderboardPage, *leaderboardIngress)
	if err != nil {
		logger.Fatal(err.Error())
	}

	if err = triviabot.Run(ctx); err != nil {
		logger.Fatal(err.Error())
	}
}
