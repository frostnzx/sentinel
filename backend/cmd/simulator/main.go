package main

import (
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	prefix := strings.Trim(os.Getenv("MQTT_TOPIC_PREFIX"), "/")
	if prefix == "" {
		prefix = "school-demo"
	}
	topic := func(s string) string { return prefix + "/" + s }
	clientID := os.Getenv("SIMULATOR_CLIENT_ID")
	if clientID == "" {
		clientID = "smart-door-simulator-local"
	}
	opts := mqtt.NewClientOptions().AddBroker(value("MQTT_URL", "tcp://localhost:1883")).SetClientID(clientID).SetAutoReconnect(true)
	client := mqtt.NewClient(opts)
	if t := client.Connect(); !t.WaitTimeout(15*time.Second) || t.Error() != nil {
		log.Fatal(t.Error())
	}
	defer client.Disconnect(250)
	locked := true
	var stateMu sync.Mutex
	executed := make(map[string][]byte)
	if t := client.Subscribe(topic("door/cmd/lock"), 1, func(_ mqtt.Client, m mqtt.Message) {
		var cmd struct {
			ID        string    `json:"id"`
			Action    string    `json:"action"`
			ExpiresAt time.Time `json:"expires_at"`
		}
		if json.Unmarshal(m.Payload(), &cmd) != nil || cmd.ID == "" {
			return
		}
		stateMu.Lock()
		if previous, ok := executed[cmd.ID]; ok {
			body := append([]byte(nil), previous...)
			stateMu.Unlock()
			client.Publish(topic("door/cmd/result"), 1, false, body).Wait()
			return
		}
		status, msg := "succeeded", cmd.Action+" command executed"
		if time.Now().After(cmd.ExpiresAt) {
			status, msg = "failed", "command expired"
		} else if cmd.Action == "lock" {
			locked = true
		} else if cmd.Action == "unlock" {
			locked = false
		} else {
			status, msg = "failed", "invalid action"
		}
		body, _ := json.Marshal(map[string]string{"id": cmd.ID, "status": status, "message": msg, "recorded_at": time.Now().UTC().Format(time.RFC3339Nano)})
		executed[cmd.ID] = body
		stateMu.Unlock()
		client.Publish(topic("door/cmd/result"), 1, false, body).Wait()
	}); !t.Wait() || t.Error() != nil {
		log.Fatal(t.Error())
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		now := time.Now().UTC()
		doorOpen, presence := false, rand.IntN(4) == 0
		stateMu.Lock()
		lock := "locked"
		if !locked {
			lock = "unlocked"
		}
		stateMu.Unlock()
		id1, id2 := uuid(), uuid()
		temperature, dist, lux := 24+rand.Float64()*2, 400+rand.Float64()*300, 80+rand.Float64()*20
		publish(client, topic("door/node/state"), map[string]any{"id": id1, "source": "node", "door_open": doorOpen, "lock_state": lock, "presence": presence, "temperature_c": temperature, "recorded_at": now})
		publish(client, topic("door/gateway/telemetry"), map[string]any{"id": id2, "source": "gateway", "distance_mm": dist, "light_lux": lux, "recorded_at": now})
		<-ticker.C
	}
}
func publish(c mqtt.Client, t string, v any) {
	b, _ := json.Marshal(v)
	if token := c.Publish(t, 1, false, b); !token.WaitTimeout(3*time.Second) || token.Error() != nil {
		log.Printf("publish %s: %v", t, token.Error())
	}
}
func uuid() string {
	b := make([]byte, 16)
	if _, e := crand.Read(b); e != nil {
		panic(e)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func value(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
