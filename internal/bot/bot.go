// Package bot ...
package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"nhooyr.io/websocket"
)

type MsgTypeFilter string

var (
	MsgFilter         MsgTypeFilter = "MSG"
	PrivMsgFilter     MsgTypeFilter = "PRIVMSG"
	NamesFilter       MsgTypeFilter = "NAMES"
	JoinFilter        MsgTypeFilter = "QUIT"
	QuitFilter        MsgTypeFilter = "JOIN"
	ViewerStateFilter MsgTypeFilter = "VIEWERSTATE"
)

type Bot struct {
	logger         *zap.SugaredLogger
	conn           *websocket.Conn
	reconnect      bool
	lastSentMsg    string
	url            string
	token          string
	filters        []MsgTypeFilter
	onMsgFuncs     []func(context.Context, *Msg) error
	onPrivMsgFuncs []func(context.Context, *Msg) error
}

type Msg struct {
	Kind string `json:"-"`
	Data string `json:"data"`
	User string `json:"nick,omitempty"`
	Time int64  `json:"timestamp,omitempty"`
}

func New(
	logger *zap.SugaredLogger,
	url, jwt string,
	reconnect bool,
	filters ...MsgTypeFilter,
) (*Bot, error) {
	b := &Bot{
		logger:    logger,
		reconnect: reconnect,
		filters:   filters,
		url:       url,
		token:     jwt,
	}
	if err := b.dial(url, jwt); err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}
	return b, nil
}

func (b *Bot) dial(url, jwt string) error {
	c, _, err := websocket.Dial(context.Background(), url,
		&websocket.DialOptions{
			HTTPHeader: http.Header{
				"Cookie": []string{fmt.Sprintf("jwt=%s", jwt)},
			},
		})
	if err != nil {
		return fmt.Errorf("failed to dial %s: %w", url, err)
	}

	b.conn = c
	b.logger.Debugw("dialed server", "url", url)
	return nil
}

func (b *Bot) Send(msg string) error {
	if msg == b.lastSentMsg {
		msg += " ."
	}

	marsha, err := json.Marshal(&Msg{
		Data: strings.ReplaceAll(html.UnescapeString(msg), "\"", "'"),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal output message: %w", err)
	}

	if err = b.send(fmt.Sprintf("MSG %s", string(marsha))); err != nil {
		return fmt.Errorf("failed to send message %q: %w", msg, err)
	}

	b.lastSentMsg = msg

	return nil
}

func (b *Bot) SendPriv(msg, user string) error {
	marsha, err := json.Marshal(&Msg{
		Data: strings.ReplaceAll(html.UnescapeString(msg), "\"", "'"),
		User: user,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal output message: %w", err)
	}

	if err = b.send(fmt.Sprintf("PRIVMSG %s", string(marsha))); err != nil {
		return fmt.Errorf("failed to priv send message %q to %q: %w", msg, user, err)
	}

	return nil
}

func (b *Bot) OnMessage(funcs ...func(context.Context, *Msg) error) {
	b.logger.Debug("setting onMessage functions")
	b.onMsgFuncs = append(b.onMsgFuncs, funcs...)
}

func (b *Bot) OnPrivMessage(funcs ...func(context.Context, *Msg) error) {
	b.logger.Debug("setting onPrivMessage functions")
	b.onPrivMsgFuncs = append(b.onPrivMsgFuncs, funcs...)
}

func (b *Bot) Run() error {
	defer func() {
		if err := b.Destroy(); err != nil {
			b.logger.Fatalf("failed to destroy bot: %v", err)
		}
	}()

	b.logger.Info("bot is now running")
	rawMsgs := make(chan string)

	eg, ctx := errgroup.WithContext(context.Background())
	eg.Go(func() error {
		defer close(rawMsgs)

		for {
			_, data, err := b.conn.Read(ctx)
			if err != nil {
				if b.reconnect {
					return b.dial(b.url, b.token)
				}
				return fmt.Errorf("failed while reading message: %w", err)
			}
			b.logger.Debugw("message read", "msg", string(data))
			rawMsgs <- string(data)
		}
	})

	eg.Go(func() error {
	OUTER:
		for raw := range rawMsgs {
			for _, filter := range b.filters {
				if strings.HasPrefix(raw, string(filter)) {
					continue OUTER
				}
			}

			msg, err := parseMsg(raw)
			if err != nil {
				b.logger.Infow("failed to parse message", "err", err)
				continue
			}

			b.logger.Debugw("parsed message", "msg", msg)
			switch msg.Kind {
			case "MSG":
				for _, f := range b.onMsgFuncs {
					if err = f(ctx, msg); err != nil {
						return fmt.Errorf("on message func err: %w", err)
					}
				}
			case "PRIVMSG":
				for _, f := range b.onPrivMsgFuncs {
					if err = f(ctx, msg); err != nil {
						return fmt.Errorf("on private message func err: %w", err)
					}
				}
			default:
			}
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("failure while running: %w", err)
	}

	return nil
}

func (b *Bot) Destroy() error {
	b.logger.Info("self destruction initiated")
	return b.conn.Close(websocket.StatusNormalClosure, "going away")
}

func parseMsg(raw string) (*Msg, error) {
	if strings.HasPrefix(raw, "ERR") {
		value, err := strconv.Unquote(raw[4:])
		if err != nil {
			return nil, fmt.Errorf("failed to unquote string: %w", err)
		}
		return nil, fmt.Errorf("server returned error: %s", value)
	}

	data := strings.SplitN(raw, " ", 2)
	var content map[string]interface{}
	var err error
	if err = json.Unmarshal([]byte(data[1]), &content); err != nil {
		return nil, fmt.Errorf("failed to unmarshal content: %v %w", data[1], err)
	}

	out := &Msg{Kind: data[0]}
	if out.Kind == "MSG" || out.Kind == "PRIVMSG" {
		if err = json.Unmarshal([]byte(data[1]), out); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message: %v %w", data[1], err)
		}
	}
	return out, nil
}

func (b *Bot) send(msg string) error {
	b.logger.Debugw("sending message", "msg", msg)
	if err := b.conn.Write(context.Background(), websocket.MessageText, []byte(msg)); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}
	return nil
}
