package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

var (
	magnetLink  = flag.String("magnet", "", "Magnetic link to download/seed")
	downloadDir = flag.String("download", "/var/lib/accumulate/downloads", "Download directory")
	dataDir     = flag.String("data", "/var/lib/accumulate/torrent-data", "Torrent data directory")
	port        = flag.Int("port", 51413, "BitTorrent listening port")
	seedAfter   = flag.Bool("seed-after", true, "Continue seeding after download completes")
)

func main() {
	flag.Parse()

	log.Println("=== Accumulate Torrent Client ===")
	log.Printf("Magnetic link: %s", *magnetLink)
	log.Printf("Download dir: %s", *downloadDir)
	log.Printf("Port: %d", *port)
	log.Printf("Seed after download: %v", *seedAfter)

	if *magnetLink == "" {
		log.Fatal("Error: -magnet flag is required")
	}

	// Create directories
	if err := os.MkdirAll(*downloadDir, 0755); err != nil {
		log.Fatalf("Error creating download directory: %v", err)
	}
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("Error creating data directory: %v", err)
	}

	// Configure torrent client
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = *dataDir
	cfg.Seed = *seedAfter
	cfg.ListenPort = *port
	cfg.NoUpload = false
	cfg.DisableTCP = false
	cfg.DisableUTP = false
	cfg.NoDHT = false

	// Create client
	client, err := torrent.NewClient(cfg)
	if err != nil {
		log.Fatalf("Error creating torrent client: %v", err)
	}
	defer client.Close()

	log.Printf("Torrent client started on port %d", *port)

	// Add torrent from magnetic link
	t, err := client.AddMagnet(*magnetLink)
	if err != nil {
		log.Fatalf("Error adding magnetic link: %v", err)
	}

	log.Printf("Added torrent from magnetic link")
	log.Printf("Waiting for metadata...")

	// Wait for torrent metadata
	<-t.GotInfo()
	info := t.Info()
	log.Printf("Got torrent info: %s (%d bytes)", t.Name(), t.Length())

	// Check if file already exists
	outputPath := filepath.Join(*downloadDir, info.BestName())
	if fileExists(outputPath) {
		log.Printf("File already exists: %s", outputPath)
		log.Printf("Skipping download, seeding existing file...")
	} else {
		log.Printf("File not found locally, downloading...")

		// Download all files
		t.DownloadAll()

		// Monitor download progress
		go monitorProgress(t)

		// Wait for download to complete
		log.Println("Waiting for download to complete...")
		for !t.Complete.Bool() {
			time.Sleep(1 * time.Second)
		}

		log.Printf("Download complete!")
		log.Printf("File saved to: %s", outputPath)
	}

	if *seedAfter {
		log.Println("Starting to seed (helping other nodes download)...")
		log.Println("Press Ctrl+C to stop seeding")

		// Monitor seeding
		go monitorSeeding(t)

		// Wait for interrupt
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down...")
	} else {
		log.Println("Download complete, not seeding (seed-after=false)")
	}

	t.Drop()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func monitorProgress(t *torrent.Torrent) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if t.Complete.Bool() {
			return
		}

		stats := t.Stats()
		bytesCompleted := t.BytesCompleted()
		bytesTotal := t.Length()
		percent := float64(bytesCompleted) / float64(bytesTotal) * 100

		log.Printf("Progress: %.1f%% (%s / %s) | Peers: %d | Download: %s/s | Upload: %s/s",
			percent,
			humanizeBytes(bytesCompleted),
			humanizeBytes(bytesTotal),
			stats.ActivePeers,
			humanizeBytes(stats.BytesReadData.Int64()/5), // Approximate per-second rate
			humanizeBytes(stats.BytesWrittenData.Int64()/5),
		)
	}
}

func monitorSeeding(t *torrent.Torrent) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := t.Stats()
		log.Printf("Seeding: Peers=%d | Uploaded=%s | Ratio=%.2f",
			stats.ActivePeers,
			humanizeBytes(stats.BytesWrittenData.Int64()),
			float64(stats.BytesWrittenData.Int64())/float64(max(stats.BytesReadData.Int64(), 1)),
		)
	}
}

func humanizeBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
