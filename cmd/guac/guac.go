package main

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/codecademy-engineering/guac"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	certPath    string
	certKeyPath string
	guacdAddr   = "127.0.0.1:4822"
)

func main() {
	// Configure the main application logger
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Configure the guac package logger separately (optional)
	// This demonstrates that the guac package has its own isolated logger
	// Uncomment to enable guac internal logging:
	guac.SetLogLevelConsole(zerolog.DebugLevel) // Use console output for development
	// guac.SetLogLevel(zerolog.InfoLevel)        // Use JSON output for production

	if os.Getenv("CERT_PATH") != "" {
		certPath = os.Getenv("CERT_PATH")
	}

	if os.Getenv("CERT_KEY_PATH") != "" {
		certKeyPath = os.Getenv("CERT_KEY_PATH")
	}

	if certPath != "" && certKeyPath == "" {
		log.Fatal().Msg("you must set the CERT_KEY_PATH environment variable to specify the full path to the certificate keyfile")
	}

	if certPath == "" && certKeyPath != "" {
		log.Fatal().Msg("you must set the CERT_PATH environment variable to specify the full path to the certificate file")
	}

	if os.Getenv("GUACD_ADDRESS") != "" {
		guacdAddr = os.Getenv("GUACD_ADDRESS")
	}

	servlet := guac.NewServer(DemoDoConnect)
	wsServer := guac.NewWebsocketServer(DemoDoConnect)

	sessions := guac.NewMemorySessionStore()
	wsServer.OnConnect = sessions.Add
	wsServer.OnDisconnect = sessions.Delete

	mux := http.NewServeMux()
	mux.Handle("/tunnel", servlet)
	mux.Handle("/tunnel/", servlet)
	mux.Handle("/websocket-tunnel", wsServer)
	mux.HandleFunc("/sessions/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		sessions.RLock()
		defer sessions.RUnlock()

		type ConnIds struct {
			Uuid string `json:"uuid"`
			Num  int    `json:"num"`
		}

		connIds := make([]*ConnIds, len(sessions.ConnIds))

		i := 0
		for id, num := range sessions.ConnIds {
			connIds[i] = &ConnIds{
				Uuid: id,
				Num:  num,
			}
		}

		if err := json.NewEncoder(w).Encode(connIds); err != nil {
			log.Error().Err(err).Msg("error encoding sessions")
		}
	})

	tlsCfg := tls.Config{}
	if certPath != "" {
		cert, err := tls.LoadX509KeyPair(certPath, certKeyPath)
		if err != nil {
			log.Fatal().Err(err).Msg("unable to load certificate keypair")
		}

		tlsCfg.MinVersion = tls.VersionTLS13
		tlsCfg.Certificates = []tls.Certificate{cert}
		tlsCfg.CurvePreferences = []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		}
	}

	s := &http.Server{
		Addr:           "0.0.0.0:4567",
		Handler:        mux,
		ReadTimeout:    guac.SocketTimeout,
		WriteTimeout:   guac.SocketTimeout,
		MaxHeaderBytes: 1 << 20,
		TLSConfig:      &tlsCfg,
	}

	if certPath != "" {
		log.Info().Msg("serving on https://0.0.0.0:4567")

		err := s.ListenAndServeTLS("", "")
		if err != nil {
			log.Fatal().Err(err).Msg("failed to start HTTPS server")
		}
	} else {
		log.Info().Msg("serving on http://0.0.0.0:4567")

		err := s.ListenAndServe()
		if err != nil {
			log.Fatal().Err(err).Msg("failed to start HTTP server")
		}
	}
}

// DemoDoConnect creates the tunnel to the remote machine (via guacd)
func DemoDoConnect(request *http.Request) (guac.Tunnel, error) {
	config := guac.NewGuacamoleConfiguration()

	var query url.Values
	if request.URL.RawQuery == "connect" {
		// http tunnel uses the body to pass parameters
		data, err := io.ReadAll(request.Body)
		if err != nil {
			log.Error().Err(err).Msg("failed to read body")
			return nil, err
		}
		_ = request.Body.Close()
		queryString := string(data)
		query, err = url.ParseQuery(queryString)
		if err != nil {
			log.Error().Err(err).Msg("failed to parse body query")
			return nil, err
		}
		log.Debug().Str("body", queryString).Interface("query", query).Msg("parsed request body")
	} else {
		query = request.URL.Query()
	}

	config.Protocol = query.Get("scheme")
	config.Parameters = map[string]string{}
	for k, v := range query {
		config.Parameters[k] = v[0]
	}

	var err error
	if query.Get("width") != "" {
		config.OptimalScreenHeight, err = strconv.Atoi(query.Get("width"))
		if err != nil || config.OptimalScreenHeight == 0 {
			log.Error().Msg("invalid height")
			config.OptimalScreenHeight = 600
		}
	}
	if query.Get("height") != "" {
		config.OptimalScreenWidth, err = strconv.Atoi(query.Get("height"))
		if err != nil || config.OptimalScreenWidth == 0 {
			log.Error().Msg("invalid width")
			config.OptimalScreenWidth = 800
		}
	}
	config.AudioMimetypes = []string{"audio/L16", "rate=44100", "channels=2"}

	log.Debug().Msg("connecting to guacd")
	addr, err := net.ResolveTCPAddr("tcp", guacdAddr)
	if err != nil {
		log.Error().Err(err).Msg("error resolving guacd address")
		return nil, err
	}

	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		log.Error().Err(err).Msg("error while connecting to guacd")
		return nil, err
	}

	stream := guac.NewStream(conn, guac.SocketTimeout)

	log.Debug().Msg("connected to guacd")
	if request.URL.Query().Get("uuid") != "" {
		config.ConnectionID = request.URL.Query().Get("uuid")
	}

	sanitisedCfg := config
	sanitisedCfg.Parameters["password"] = "********"
	log.Debug().Interface("config", sanitisedCfg).Msg("starting handshake")
	err = stream.Handshake(config)
	if err != nil {
		return nil, err
	}
	log.Debug().Msg("socket configured")
	return guac.NewSimpleTunnel(stream), nil
}
