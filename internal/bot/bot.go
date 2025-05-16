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

	"github.com/coder/websocket"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type MsgTypeFilter string

var (
	MsgFilter         MsgTypeFilter = "MSG"
	PrivMsgFilter     MsgTypeFilter = "PRIVMSG"
	PrivMsgSentFilter MsgTypeFilter = "PRIVMSGSENT"
	NamesFilter       MsgTypeFilter = "NAMES"
	JoinFilter        MsgTypeFilter = "QUIT"
	QuitFilter        MsgTypeFilter = "JOIN"
	ViewerStateFilter MsgTypeFilter = "VIEWERSTATE"
)

type Bot struct {
	logger         *zap.SugaredLogger
	conn           WebsocketConn
	dialer         Dialer
	reconnect      bool
	lastSentMsg    string
	url            string
	token          string
	filters        []MsgTypeFilter
	onMsgFuncs     []func(context.Context, *Msg) error
	onPrivMsgFuncs []func(context.Context, *Msg) error
}

type Msg struct {
	Kind     string   `json:"-"`
	Data     string   `json:"data"`
	User     string   `json:"nick,omitempty"`
	Time     int64    `json:"timestamp,omitempty"`
	Features []string `json:"features,omitempty"`
}

type WebsocketConn interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Write(ctx context.Context, messageType websocket.MessageType, data []byte) error
	Close(code websocket.StatusCode, reason string) error
}

type Dialer func(ctx context.Context, url string, opts *websocket.DialOptions) (WebsocketConn, *http.Response, error)

func WebSocketDialer(ctx context.Context, url string, opts *websocket.DialOptions) (WebsocketConn, *http.Response, error) {
	return websocket.Dial(ctx, url, opts)
}

func New(
	ctx context.Context,
	logger *zap.SugaredLogger,
	dialer Dialer,
	url, jwt string,
	reconnect bool,
	filters ...MsgTypeFilter,
) (*Bot, error) {
	b := &Bot{
		logger:    logger,
		dialer:    dialer,
		reconnect: reconnect,
		filters:   filters,
		url:       url,
		token:     jwt,
	}
	if err := b.dial(ctx, url, jwt); err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}
	return b, nil
}

func (b *Bot) dial(ctx context.Context, url, jwt string) error {
	c, _, err := b.dialer(
		ctx,
		url,
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

func (b *Bot) Send(ctx context.Context, msg string) error {
	if msg == b.lastSentMsg {
		msg += " ."
	}

	marsha, err := json.Marshal(&Msg{
		Data: strings.ReplaceAll(html.UnescapeString(msg), "\"", "'"),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal output message: %w", err)
	}

	if err = b.send(ctx, fmt.Sprintf("MSG %s", string(marsha))); err != nil {
		return fmt.Errorf("failed to send message %q: %w", msg, err)
	}

	b.lastSentMsg = msg

	return nil
}

func (b *Bot) SendPriv(ctx context.Context, msg, user string) error {
	marsha, err := json.Marshal(&Msg{
		Data: strings.ReplaceAll(html.UnescapeString(msg), "\"", "'"),
		User: user,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal output message: %w", err)
	}

	if err = b.send(ctx, fmt.Sprintf("PRIVMSG %s", string(marsha))); err != nil {
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

func (b *Bot) Run(ctx context.Context) error {
	defer func() {
		if err := b.Destroy(); err != nil {
			b.logger.Fatalf("failed to destroy bot: %v", err)
		}
	}()

	b.logger.Info("bot is now running")
	rawMsgs := make(chan string)

	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		defer close(rawMsgs)

		for {
			select {
			case <-egCtx.Done():
				b.logger.Info("context canceled, stopping read loop")
				return nil
			default:
				_, data, err := b.conn.Read(egCtx)
				if err != nil {
					if egCtx.Err() != nil {
						// Context was canceled, exit gracefully
						return nil
					}
					if b.reconnect {
						return b.dial(ctx, b.url, b.token)
					}
					return fmt.Errorf("failed while reading message: %w", err)
				}
				b.logger.Debugw("message read", "msg", string(data))
				rawMsgs <- string(data)
			}
		}
	})

	eg.Go(func() error {
	OUTER:
		for {
			select {
			case <-egCtx.Done():
				b.logger.Info("context canceled, stopping process loop")
				return nil
			case raw, ok := <-rawMsgs:
				if !ok {
					b.logger.Info("raw messages channel closed")
					return nil
				}

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
						if err = f(egCtx, msg); err != nil {
							return fmt.Errorf("on message func err: %w", err)
						}
					}
				case "PRIVMSG":
					for _, f := range b.onPrivMsgFuncs {
						if err = f(egCtx, msg); err != nil {
							return fmt.Errorf("on private message func err: %w", err)
						}
					}
				default:
				}
			}
		}
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

func (b *Bot) send(ctx context.Context, msg string) error {
	b.logger.Debugw("sending message", "msg", msg)
	if err := b.conn.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}
	return nil
}

func (m Msg) IsMod() bool {
	for _, feature := range m.Features {
		if feature == "moderator" {
			return true
		}
	}
	return false
}
