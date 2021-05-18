package main

import (
	"log"
	"os"

	"github.com/jbpratt/trivia/internal/triviabot"
	"go.uber.org/zap"
)

const devurl = "wss://chat2.strims.gg/ws"

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Sync()

	url, jwt := os.Getenv("STRIMS_CHAT_WSS_URL"), os.Getenv("STRIMS_CHAT_TOKEN")
	if url == "" {
		url = devurl
	}
	if jwt == "" {
		logger.Fatal("must provide $STRIMS_CHAT_TOKEN")
	}

	triviabot, err := triviabot.New(logger.Sugar(), url, jwt, ".", 3, 15)
	if err != nil {
		logger.Fatal(err.Error())
	}

	if err = triviabot.Run(); err != nil {
		logger.Fatal(err.Error())
	}
}

// off by one for report
// send msg for quiz completion with results
// accept duration and category and difficulty
// show time difference in between pole positions
// round number
