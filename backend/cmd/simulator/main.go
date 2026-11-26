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
		} else if cmd.Action != "lock" && cmd.Action != "unlock" {
			status, msg = "failed", "invalid action"
		}
		body, _ := json.Marshal(map[string]string{"id": cmd.ID, "status": status, "message": msg, "recorded_at": time.Now().UTC().Format(time.RFC3339Nano)})
		executed[cmd.ID] = body
		stateMu.Unlock()
		client.Publish(topic("door/cmd/result"), 1, false, body).Wait()
	}); !t.Wait() || t.Error() != nil {
		log.Fatal(t.Error())
	}
	if t := client.Subscribe(topic("door/faces/sync"), 1, func(_ mqtt.Client, message mqtt.Message) {
		var gallery struct {
			Version int64 `json:"version"`
		}
		if json.Unmarshal(message.Payload(), &gallery) == nil && gallery.Version > 0 {
			publish(client, topic("door/faces/status"), map[string]any{"version": gallery.Version, "status": "applied"})
		}
	}); !t.Wait() || t.Error() != nil {
		log.Fatal(t.Error())
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	started := time.Now()
	espBoot, piBoot := uuid(), uuid()
	var espSeq, piSeq int64
	for {
		now := time.Now().UTC()
		motion, vibration := rand.IntN(4) == 0, rand.IntN(8) == 0
		for _, sensor := range []struct {
			name  string
			value bool
		}{{"motion", motion}, {"door", true}, {"vibration", vibration}} {
			espSeq++
			body := map[string]any{"device_id": "esp32-simulator", "boot_id": espBoot, "seq": espSeq, "uptime_ms": time.Since(started).Milliseconds(), "simulated": true, "value": sensor.value, "valid": true}
			publish(client, topic("sensor/"+sensor.name), body)
		}
		for _, sensor := range []struct {
			name  string
			value float64
			unit  string
		}{{"distance", 400 + rand.Float64()*300, "mm"}, {"light", 80 + rand.Float64()*20, "lux"}} {
			piSeq++
			publish(client, topic("sensor/"+sensor.name), map[string]any{"device_id": "pi5-simulator", "boot_id": piBoot, "seq": piSeq, "ts": now, "uptime_ms": time.Since(started).Milliseconds(), "simulated": true, "value": sensor.value, "valid": true, "unit": sensor.unit})
		}
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
