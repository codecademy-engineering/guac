package guac

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// WebsocketServer implements a websocket-based connection to guacd.
type WebsocketServer struct {
	connect   func(*http.Request) (Tunnel, error)
	connectWs func(*websocket.Conn, *http.Request) (Tunnel, error)

	// OnConnect is an optional callback called when a websocket connects.
	// Deprecated: use OnConnectWs
	OnConnect func(string, *http.Request)
	// OnDisconnect is an optional callback called when the websocket disconnects.
	// Deprecated: use OnDisconnectWs
	OnDisconnect func(string, *http.Request, Tunnel)

	// OnConnectWs is an optional callback called when a websocket connects.
	OnConnectWs func(string, *websocket.Conn, *http.Request)
	// OnDisconnectWs is an optional callback called when the websocket disconnects.
	OnDisconnectWs func(string, *websocket.Conn, *http.Request, Tunnel)

	// logger is an optional logger to use for logging. If not set, the package-level s.logger will be used.
	logger *zerolog.Logger
}

// NewWebsocketServer creates a new server with a simple connect method.
func NewWebsocketServer(connect func(*http.Request) (Tunnel, error), logger *zerolog.Logger) *WebsocketServer {
	serverLogger := &globalLogger

	if logger != nil {
		serverLogger = logger
	}

	return &WebsocketServer{
		connect: connect,
		logger:  serverLogger,
	}
}

// NewWebsocketServerWs creates a new server with a connect method that takes a websocket.
func NewWebsocketServerWs(connect func(*websocket.Conn, *http.Request) (Tunnel, error), logger *zerolog.Logger) *WebsocketServer {
	serverLogger := &globalLogger

	if logger != nil {
		serverLogger = logger
	}

	return &WebsocketServer{
		connectWs: connect,
		logger:    serverLogger,
	}
}

const (
	websocketReadBufferSize  = MaxGuacMessage
	websocketWriteBufferSize = MaxGuacMessage * 2
)

func (s *WebsocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  websocketReadBufferSize,
		WriteBufferSize: websocketWriteBufferSize,
		CheckOrigin: func(r *http.Request) bool {
			return true // TODO
		},
	}
	protocol := r.Header.Get("Sec-Websocket-Protocol")
	ws, err := upgrader.Upgrade(w, r, http.Header{
		"Sec-Websocket-Protocol": {protocol},
	})
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to upgrade websocket")
		return
	}
	defer func() {
		if err = ws.Close(); err != nil {
			s.logger.Trace().Err(err).Msg("Error closing websocket")
		}
	}()

	s.logger.Trace().Msg("connecting to tunnel")
	var tunnel Tunnel
	var e error
	if s.connect != nil {
		tunnel, e = s.connect(r)
	} else {
		tunnel, e = s.connectWs(ws, r)
	}
	if e != nil {
		return
	}
	defer func() {
		if err = tunnel.Close(); err != nil {
			s.logger.Trace().Err(err).Msg("Error closing tunnel")
		}
	}()
	s.logger.Trace().Msg("connected to tunnel")

	id := tunnel.ConnectionID()

	// Enhance logger with connection ID context
	s.logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Str("connection_id", id)
	})

	s.logger.Trace().Str("connection_id", id).Str("remote_addr", r.RemoteAddr).Msg("websocket connection established")

	if s.OnConnect != nil {
		s.OnConnect(id, r)
	}
	if s.OnConnectWs != nil {
		s.OnConnectWs(id, ws, r)
	}

	writer := tunnel.AcquireWriter()
	reader := tunnel.AcquireReader()

	if s.OnDisconnect != nil {
		defer s.OnDisconnect(id, r, tunnel)
	}
	if s.OnDisconnectWs != nil {
		defer s.OnDisconnectWs(id, ws, r, tunnel)
	}
	defer s.logger.Trace().Str("connection_id", id).Msg("websocket connection closed")

	defer tunnel.ReleaseWriter()
	defer tunnel.ReleaseReader()

	go wsToGuacd(s.logger, ws, writer)
	guacdToWs(s.logger, ws, reader)
}

// MessageReader wraps a websocket connection and only permits Reading
type MessageReader interface {
	// ReadMessage should return a single complete message to send to guac
	ReadMessage() (int, []byte, error)
}

func wsToGuacd(logger *zerolog.Logger, ws MessageReader, guacd io.Writer) {
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			logger.Trace().Err(err).Msg("Error reading message from ws")
			logger.Warn().Err(err).Msg("[Browser -> guacd] Browser disconnected or error reading from WebSocket")
			return
		}

		if bytes.HasPrefix(data, internalOpcodeIns) {
			// messages starting with the InternalDataOpcode are never sent to guacd
			continue
		}

		if _, err = guacd.Write(data); err != nil {
			logger.Trace().Err(err).Msg("Failed writing to guacd")
			logger.Error().Err(err).Msg("[Browser -> guacd] Failed to write to guacd (guacd may have disconnected)")
			return
		}
	}
}

// MessageWriter wraps a websocket connection and only permits Writing
type MessageWriter interface {
	// WriteMessage writes one or more complete guac commands to the websocket
	WriteMessage(int, []byte) error
}

func guacdToWs(logger *zerolog.Logger, ws MessageWriter, guacd InstructionReader) {
	buf := bytes.NewBuffer(make([]byte, 0, MaxGuacMessage*2))

	for {
		ins, err := guacd.ReadSome()
		if err != nil {
			logger.Warn().Err(err).Msg("[guacd -> Browser] guacd disconnected or error reading from guacd")
			return
		}

		if bytes.HasPrefix(ins, internalOpcodeIns) {
			// messages starting with the InternalDataOpcode are never sent to the websocket
			continue
		}

		if _, err = buf.Write(ins); err != nil {
			logger.Error().Err(err).Msg("[guacd -> Browser] Failed to buffer message from guacd")
			return
		}

		// if the buffer has more data in it or we've reached the max buffer size, send the data and reset
		if !guacd.Available() || buf.Len() >= MaxGuacMessage {
			if err = ws.WriteMessage(1, buf.Bytes()); err != nil {
				if err == websocket.ErrCloseSent {
					logger.Debug().Msg("[guacd -> Browser] websocket already closed (clean close)")
					return
				}
				logger.Warn().Err(err).Msg("[guacd -> Browser] Failed to write to WebSocket (browser may have disconnected)")
				return
			}
			buf.Reset()
		}
	}
}
