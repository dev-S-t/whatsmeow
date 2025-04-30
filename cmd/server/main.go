// cmd/server/main.go
package main

import (
  "context"
  "fmt"
  "net/http"
  "os"
  "time"

  "go.mau.fi/whatsmeow"
  "go.mau.fi/whatsmeow/store/sqlstore"
  "go.mau.fi/whatsmeow/types/events"
  waLog "go.mau.fi/whatsmeow/util/log"

  "github.com/gin-gonic/gin"
)

func main() {
  // 1) Init DB-backed device store (SQLite for simplicity)
  dbLog := waLog.Stdout("DB", "DEBUG", true)
  container, err := sqlstore.New("sqlite3", "file:store.db?_foreign_keys=on", dbLog)
  if err != nil { panic(err) }
  device, err := container.GetFirstDevice()
  if err != nil { panic(err) }

  // 2) Create WhatsApp client
  clientLog := waLog.Stdout("WA", "DEBUG", true)
  client := whatsmeow.NewClient(device, clientLog)

  // 3) In-memory channel for SSE
  msgChan := make(chan string)

  // 4) Event handler pushes incoming text to msgChan
  client.AddEventHandler(func(evt interface{}) {
    if m, ok := evt.(*events.Message); ok {
      if txt := m.Message.GetConversation(); txt != "" {
        msgChan <- fmt.Sprintf("%s: %s", m.Info.PushName, txt)
      }
    }
  })

  // 5) Connect / login (will print QR code on first run)
  if client.Store.ID == nil {
    qrChan, _ := client.GetQRChannel(context.Background())
    go func() {
      if err := client.Connect(); err != nil { panic(err) }
    }()
    for evt := range qrChan {
      if evt.Event == "code" {
        fmt.Println("QR code:", evt.Code)
      }
    }
  } else {
    if err := client.Connect(); err != nil { panic(err) }
  }

  // 6) Start Gin on $PORT, host 0.0.0.0
  r := gin.Default()
  port := os.Getenv("PORT")
  if port == "" { port = "10000" } // Render’s default :contentReference[oaicite:0]{index=0}

  // POST /send → { to, message }
  r.POST("/send", func(c *gin.Context) {
    var req struct {
      To      string `json:"to"`
      Message string `json:"message"`
    }
    if err := c.BindJSON(&req); err != nil {
      c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
      return
    }
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    _, err := client.SendMessage(ctx, req.To, &waProto.Message{Conversation: req.Message})
    if err != nil {
      c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
      return
    }
    c.JSON(http.StatusOK, gin.H{"status": "sent"})
  })

  // GET /receive → SSE stream
  r.GET("/receive", func(c *gin.Context) {
    c.Writer.Header().Set("Content-Type", "text/event-stream")
    c.Writer.Header().Set("Cache-Control", "no-cache")
    for msg := range msgChan {
      fmt.Fprintf(c.Writer, "data: %s\n\n", msg)
      c.Writer.Flush()
    }
  })

  r.Run("0.0.0.0:" + port)
}
