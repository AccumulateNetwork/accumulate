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
	volumesFile = flag.String("file", "/media/paul/Expansion/accumulate-blockchain/bvn0-production/compressed/BVN0_CLEAN_20241011.tar.gz", "Path to volumes .tar.gz file")
	port        = flag.Int("port", 51413, "BitTorrent listening port")
	dataDir     = flag.String("data", "./torrent-data", "Data directory for torrent files")
	seedOnly    = flag.Bool("seed-only", true, "Only seed, don't download")
)

func main() {
	flag.Parse()

	log.Println("=== Accumulate Torrent Server ===")
	log.Printf("Volumes file: %s", *volumesFile)
	log.Printf("Port: %d", *port)
	log.Printf("Data directory: %s", *dataDir)

	// Verify volumes file exists
	if _, err := os.Stat(*volumesFile); os.IsNotExist(err) {
		log.Fatalf("Error: Volumes file not found: %s", *volumesFile)
	}

	// Create data directory
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("Error creating data directory: %v", err)
	}

	// Configure torrent client
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = *dataDir
	cfg.Seed = true
	cfg.ListenPort = *port
	cfg.NoUpload = false
	cfg.DisableTCP = false
	cfg.DisableUTP = false
	cfg.DisableIPv6 = false

	// Enable DHT for trackerless peer discovery
	cfg.NoDHT = false

	// Create client
	client, err := torrent.NewClient(cfg)
	if err != nil {
		log.Fatalf("Error creating torrent client: %v", err)
	}
	defer client.Close()

	log.Printf("Torrent client started on port %d", *port)

	// Create or load torrent file
	torrentFile := filepath.Join(*dataDir, filepath.Base(*volumesFile)+".torrent")
	var mi *metainfo.MetaInfo

	if _, err := os.Stat(torrentFile); os.IsNotExist(err) {
		log.Printf("Creating torrent file for: %s", *volumesFile)
		mi, err = createTorrent(*volumesFile, torrentFile)
		if err != nil {
			log.Fatalf("Error creating torrent: %v", err)
		}
	} else {
		log.Printf("Loading existing torrent file: %s", torrentFile)
		mi, err = metainfo.LoadFromFile(torrentFile)
		if err != nil {
			log.Fatalf("Error loading torrent file: %v", err)
		}
	}

	// Display magnetic link
	info, err := mi.UnmarshalInfo()
	if err != nil {
		log.Fatalf("Error unmarshaling info: %v", err)
	}
	magnetLink := mi.Magnet(nil, &info).String()
	log.Println("")
	log.Println("=== MAGNETIC LINK ===")
	log.Println(magnetLink)
	log.Println("=====================")
	log.Println("")

	// Save magnetic link to file
	magnetFile := filepath.Join(*dataDir, filepath.Base(*volumesFile)+".magnet")
	if err := os.WriteFile(magnetFile, []byte(magnetLink), 0644); err != nil {
		log.Printf("Warning: Could not save magnet link: %v", err)
	} else {
		log.Printf("Magnetic link saved to: %s", magnetFile)
	}

	// Add torrent to client
	t, err := client.AddTorrent(mi)
	if err != nil {
		log.Fatalf("Error adding torrent: %v", err)
	}

	log.Printf("Added torrent: %s", t.Name())
	log.Printf("Info hash: %s", t.InfoHash().HexString())

	// Start seeding
	t.DownloadAll()

	if *seedOnly {
		log.Println("Seed-only mode: will not download, only seed existing file")
	}

	// Wait for torrent metadata
	<-t.GotInfo()
	log.Printf("Got torrent info: %s (%d bytes)", t.Name(), t.Length())

	// Display seeding status
	go func() {
		for range time.Tick(30 * time.Second) {
			stats := t.Stats()
			log.Printf("Status: Peers=%d Upload=%s Download=%s Ratio=%.2f",
				stats.ActivePeers,
				humanizeBytes(stats.BytesWrittenData.Int64()),
				humanizeBytes(stats.BytesReadData.Int64()),
				float64(stats.BytesWrittenData.Int64())/float64(max(stats.BytesReadData.Int64(), 1)),
			)
		}
	}()

	log.Println("Seeding... Press Ctrl+C to stop")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	t.Drop()
}

func createTorrent(filePath, torrentPath string) (*metainfo.MetaInfo, error) {
	log.Println("Generating .torrent file (this may take a while)...")

	mi := metainfo.MetaInfo{
		AnnounceList: [][]string{
			{"udp://tracker.opentrackr.org:1337/announce"},
			{"udp://tracker.openbittorrent.com:6969/announce"},
			{"udp://9.rarbg.com:2810/announce"},
			{"udp://tracker.torrent.eu.org:451/announce"},
		},
	}

	info := metainfo.Info{
		PieceLength: 16 * 1024 * 1024, // 16 MB pieces
	}

	err := info.BuildFromFilePath(filePath)
	if err != nil {
		return nil, fmt.Errorf("build from file: %w", err)
	}

	mi.InfoBytes, err = bencode.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("marshal info: %w", err)
	}

	// Save torrent file
	f, err := os.Create(torrentPath)
	if err != nil {
		return nil, fmt.Errorf("create torrent file: %w", err)
	}
	defer f.Close()

	if err := mi.Write(f); err != nil {
		return nil, fmt.Errorf("write torrent file: %w", err)
	}

	log.Printf("Torrent file created: %s", torrentPath)
	return &mi, nil
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
