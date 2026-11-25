package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type commandMQTT struct {
	mqtt.Client
	connected bool
	confirmed bool
	err       error
	published int
	topic     string
	qos       byte
	retained  bool
	payload   []byte
}

func (m *commandMQTT) IsConnected() bool { return m.connected }
func (m *commandMQTT) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	m.published++
	m.topic, m.qos, m.retained = topic, qos, retained
	m.payload = payload.([]byte)
	return commandToken{confirmed: m.confirmed, err: m.err}
}

type commandToken struct {
	mqtt.Token
	confirmed bool
	err       error
}

func (t commandToken) WaitTimeout(time.Duration) bool { return t.confirmed }
func (t commandToken) Error() error                   { return t.err }

func TestCreateCommandSendsWithoutDeviceResult(t *testing.T) {
	for _, tc := range []struct {
		name       string
		readingAge time.Duration
		connected  bool
		confirmed  bool
		err        error
		wantCode   int
		wantCount  int
	}{
		{"published with fresh reading", time.Second, true, true, nil, http.StatusAccepted, 1},
		{"published without readings", 0, true, true, nil, http.StatusAccepted, 1},
		{"published with stale reading", time.Minute, true, true, nil, http.StatusAccepted, 1},
		{"broker disconnected", 0, false, true, nil, http.StatusServiceUnavailable, 0},
		{"broker timeout", 0, true, false, nil, http.StatusServiceUnavailable, 1},
		{"broker error", 0, true, true, errors.New("publish failed"), http.StatusServiceUnavailable, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &commandMQTT{connected: tc.connected, confirmed: tc.confirmed, err: tc.err}
			app := &App{mqtt: client, prefix: "test-prefix", latest: map[string]StoredReading{}}
			if tc.readingAge != 0 {
				app.latest["door"] = StoredReading{Source: "node", ReceivedAt: time.Now().UTC().Add(-tc.readingAge)}
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/door-01/commands", strings.NewReader(`{"action":"lock"}`))
			response := httptest.NewRecorder()
			app.createCommand(response, request)
			if response.Code != tc.wantCode || client.published != tc.wantCount {
				t.Fatalf("code=%d publishes=%d body=%s", response.Code, client.published, response.Body.String())
			}
			if tc.wantCode != http.StatusAccepted {
				return
			}
			if response.Body.String() != "{\"message\":\"Command sent\"}\n" || client.topic != "test-prefix/door/cmd/lock" || client.qos != 1 || client.retained {
				t.Fatalf("unexpected response or MQTT publish: %s %+v", response.Body.String(), client)
			}
			var payload struct {
				ID        string    `json:"id"`
				Action    string    `json:"action"`
				Origin    string    `json:"origin"`
				ExpiresAt time.Time `json:"expires_at"`
			}
			if err := json.Unmarshal(client.payload, &payload); err != nil || payload.ID == "" || payload.Action != "lock" || payload.Origin != "manual" || time.Until(payload.ExpiresAt) < 8*time.Second || time.Until(payload.ExpiresAt) > 10*time.Second {
				t.Fatalf("unexpected command payload: %s (%v)", client.payload, err)
			}
		})
	}
}
