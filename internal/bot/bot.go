// Package bot ...
package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"nhooyr.io/websocket"
)

type Bot struct {
	logger         *zap.SugaredLogger
	conn           *websocket.Conn
	msgs           chan Msg
	onMsgFuncs     []func(context.Context, *Msg) error
	onPrivMsgFuncs []func(context.Context, *Msg) error
}

type Msg struct {
	Kind string `json:"-"`
	Data string `json:"data"`
	User string `json:"nick,omitempty"`
	Time int64  `json:"timestamp,omitempty"`
}

func New(logger *zap.SugaredLogger, url, jwt string) (*Bot, error) {
	c, _, err := websocket.Dial(context.Background(), url,
		&websocket.DialOptions{
			HTTPHeader: http.Header{
				"Cookie": []string{fmt.Sprintf("jwt=%s", jwt)},
			},
		})
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %v", url, err)
	}

	logger.Debugw("dialed server", "url", url)
	return &Bot{logger, c, nil, nil, nil}, nil
}

func (b *Bot) Send(msg string) error {
	marsha, err := json.Marshal(&Msg{
		Data: strings.Replace(html.UnescapeString(msg), "\"", "'", -1),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal output msg: %v", err)
	}

	if err = b.send(fmt.Sprintf("MSG %s", string(marsha))); err != nil {
		return err
	}

	return nil
}

func (b *Bot) SendPriv(msg, user string) error {
	marsha, err := json.Marshal(&Msg{
		Data: strings.Replace(html.UnescapeString(msg), "\"", "'", -1),
		User: user,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal output msg: %v", err)
	}

	if err = b.send(fmt.Sprintf("PRIVMSG %s", string(marsha))); err != nil {
		return err
	}

	return nil
}

func (b *Bot) send(msg string) error {
	b.logger.Debugw("sending msg", "msg", msg)
	if err := b.conn.Write(context.Background(), websocket.MessageText, []byte(msg)); err != nil {
		return fmt.Errorf("failed to write message: %v", err)
	}
	return nil
}

func (b *Bot) OnMessage(funcs ...func(context.Context, *Msg) error) {
	b.logger.Debug("configuring onMessage functions")
	b.onMsgFuncs = append(b.onMsgFuncs, funcs...)
}

func (b *Bot) OnPrivMessage(funcs ...func(context.Context, *Msg) error) {
	b.logger.Debug("configuring onPrivMessage functions")
	b.onPrivMsgFuncs = append(b.onPrivMsgFuncs, funcs...)
}

func (b *Bot) Run() error {
	defer b.Destroy()

	b.logger.Info("bot is now running")
	rawMsgs := make(chan string)

	eg, ctx := errgroup.WithContext(context.Background())
	eg.Go(func() error {
		defer close(rawMsgs)

		for {
			_, data, err := b.conn.Read(ctx)
			if err != nil {
				return err
			}
			b.logger.Debugw("message read", "msg", string(data))
			rawMsgs <- string(data)
		}
	})

	eg.Go(func() error {
		for raw := range rawMsgs {
			msg, err := parseMsg(raw)
			if err != nil {
				b.logger.Infow("failed to parse msg", "err", err)
				continue
			}

			b.logger.Debugw("parsed msg", "msg", msg)
			if msg.Kind == "MSG" {
				for _, f := range b.onMsgFuncs {
					if err = f(ctx, msg); err != nil {
						return fmt.Errorf("on message func err: %v", err)
					}
				}
			} else if msg.Kind == "PRIVMSG" {
				for _, f := range b.onPrivMsgFuncs {
					if err = f(ctx, msg); err != nil {
						return fmt.Errorf("on private message func err: %v", err)
					}
				}
			}
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("failure while running: %v", err)
	}

	return nil
}

func (b *Bot) Destroy() error {
	b.logger.Info("self destruction initiated")
	return b.conn.Close(websocket.StatusNormalClosure, "going away")
}

func parseMsg(raw string) (*Msg, error) {
	data := strings.SplitN(raw, " ", 2)
	var content map[string]interface{}
	var err error
	if err = json.Unmarshal([]byte(data[1]), &content); err != nil {
		return nil, fmt.Errorf("failed to unmarshal content: %v %v", data[1], err)
	}

	out := &Msg{Kind: data[0]}
	if out.Kind == "MSG" || out.Kind == "PRIVMSG" {
		if err = json.Unmarshal([]byte(data[1]), out); err != nil {
			return nil, fmt.Errorf("failed to unmarshal msg: %v %v", data[1], err)
		}
	}
	return out, nil
}
