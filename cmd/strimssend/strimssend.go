package main

import (
	"flag"
	"log"
	"os"

	"github.com/jbpratt/bots/internal/strimssend"
	"go.uber.org/zap"
)

func main() {
	serverURL := "wss://chat.strims.gg/ws"
	dev := flag.Bool("dev", false, "use chat2")
	msg := flag.String("msg", "", "message to send")

	flag.Parse()

	if *dev {
		serverURL = "wss://chat2.strims.gg/ws"
	}

	if *msg == "" {
		log.Fatal("-msg flag is required")
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		_ = logger.Sync()
	}()

	url, jwt := os.Getenv("STRIMS_CHAT_WSS_URL"), os.Getenv("STRIMS_CHAT_TOKEN")
	if url == "" {
		url = serverURL
	}
	if jwt == "" {
		logger.Fatal("must provide $STRIMS_CHAT_TOKEN")
	}

	ss, err := strimssend.New(logger.Sugar(), url, jwt)
	if err != nil {
		log.Fatal(err)
	}

	if err = ss.Send(*msg); err != nil {
		log.Fatal(err)
	}
}
