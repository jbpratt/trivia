package bot

import (
	"context"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/mock"
)

type MockWebSocketConn struct {
	mock.Mock
}

func (m *MockWebSocketConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	args := m.Called(ctx)
	return args.Get(0).(websocket.MessageType), args.Get(1).([]byte), args.Error(2)
}

func (m *MockWebSocketConn) Write(ctx context.Context, messageType websocket.MessageType, data []byte) error {
	args := m.Called(ctx, messageType, data)
	return args.Error(0)
}

func (m *MockWebSocketConn) Close(code websocket.StatusCode, reason string) error {
	args := m.Called(code, reason)
	return args.Error(0)
}
