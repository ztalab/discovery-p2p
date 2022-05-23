package cli

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DataDrake/cli-ng/v2/cmd"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/ztalab/discovery-p2p/config"
	"github.com/ztalab/discovery-p2p/p2p"
)

var (
	// RevLookup allow quick lookups of an incoming stream
	// for security before accepting or responding to any data.
	RevLookup map[string]string
)

// Up creates and brings up a Discovery P2P Interface.
var Up = cmd.Sub{
	Name:  "up",
	Alias: "up",
	Short: "Create and Bring Up a Discovery P2P Interface.",
	Args:  &UpArgs{},
	Flags: &UpFlags{},
	Run:   UpRun,
}

// UpArgs handles the specific arguments for the up command.
type UpArgs struct {
	InterfaceName string
}

// UpFlags handles the specific flags for the up command.
type UpFlags struct {
	Pub bool `short:"p" long:"pub" desc:"pub"`
}

// UpRun handles the execution of the up command.
func UpRun(r *cmd.Root, c *cmd.Sub) {
	// Parse Command Args
	args := c.Args.(*UpArgs)

	// Parse Command Flags
	flags := c.Flags.(*UpFlags)

	// Parse Global Config Flag for Custom Config Path
	configPath := r.Flags.(*GlobalFlags).Config
	if configPath == "" {
		configPath = "./" + args.InterfaceName + ".yaml"
	}

	// Read in configuration from file.
	cfg, err := config.Read(configPath)
	checkErr(err)

	fmt.Println("[+] Creating TUN Device")

	// Setup System Context
	ctx := context.Background()

	// Check that the listener port is available.
	port, err := verifyPort(cfg.Interface.ListenPort)
	checkErr(err)
	fmt.Println("[+] Creating LibP2P Node On:" + strconv.Itoa(port))
	// Create P2P Node
	host, dht, err := p2p.CreateNode(
		ctx,
		cfg.Interface.PrivateKey,
		port,
	)
	checkErr(err)

	// Setup Peer Table for Quick Packet --> Dest ID lookup
	peerTable := make(map[string]peer.ID)
	for _, id := range cfg.Peers {
		peerTable[id], err = peer.Decode(id)
		checkErr(err)
	}

	fmt.Println("[+] Setting Up Node Discovery via DHT")

	// Setup P2P Discovery
	go p2p.Discover(ctx, host, dht, peerTable)
	go prettyDiscovery(ctx, host, peerTable)

	// Register the application to listen for SIGINT/SIGTERM
	go signalExit(host)

	fmt.Println("[+] Network Setup Complete...Waiting on Node Discovery")

	// create a new PubSub service using the GossipSub router
	ps, err := pubsub.NewGossipSub(ctx, host)
	if err != nil {
		panic(err)
	}

	topic, err := ps.Join((topicName("p2pdb")))
	if err != nil {
		panic(err)
	}
	if flags.Pub {
		for {
			time.Sleep(1 * time.Second)
			input := DataMessage{
				Message:    "bbb",
				SenderID:   "111",
				SenderNick: "222",
			}

			msgBytes, err := json.Marshal(input)
			if err != nil {
				panic(err)
			}
			topic.Publish(ctx, msgBytes)
		}
	} else {
		// and subscribe to it
		sub, err := topic.Subscribe()
		if err != nil {
			panic(err)
		}
		for {
			time.Sleep(1 * time.Second)
			msg, err := sub.Next(ctx)
			if err != nil {
				continue
			}
			// only forward messages delivered by others

			cm := new(DataMessage)
			err = json.Unmarshal(msg.Data, cm)
			log.Printf("message: %s", cm.Message)
			if err != nil {
				continue
			}
			log.Printf("cm: %v", cm)
		}

	}
}

// ChatMessage gets converted to/from JSON and sent in the body of pubsub messages.
type DataMessage struct {
	Message    string
	SenderID   string
	SenderNick string
}

func topicName(Name string) string {
	return "chat-room:" + Name
}

// singalExit registers two syscall handlers on the system  so that if
// an SIGINT or SIGTERM occur on the system discovery-p2p can gracefully
// shutdown and remove the filesystem lock file.
func signalExit(host host.Host) {
	// Wait for a SIGINT or SIGTERM signal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch

	// Shut the node down
	err := host.Close()
	checkErr(err)

	fmt.Println("Received signal, shutting down...")

	// Exit the application.
	os.Exit(0)
}

func streamHandler(stream network.Stream) {
	// If the remote node ID isn't in the list of known nodes don't respond.
	if _, ok := RevLookup[stream.Conn().RemotePeer().Pretty()]; !ok {
		stream.Reset()
		return
	}
	var packet = make([]byte, 1420)
	var packetSize = make([]byte, 2)
	for {
		// Read the incoming packet's size as a binary value.
		_, err := stream.Read(packetSize)
		if err != nil {
			stream.Close()
			return
		}

		// Decode the incoming packet's size from binary.
		size := binary.LittleEndian.Uint16(packetSize)

		// Read in the packet until completion.
		var plen uint16 = 0
		for plen < size {
			tmp, err := stream.Read(packet[plen:size])
			plen += uint16(tmp)
			if err != nil {
				stream.Close()
				return
			}
		}
		fmt.Println("rev: " + string(packet[:size]))
	}
}

func prettyDiscovery(ctx context.Context, node host.Host, peerTable map[string]peer.ID) {
	// Build a temporary map of peers to limit querying to only those
	// not connected.
	tempTable := make(map[string]peer.ID, len(peerTable))
	for ip, id := range peerTable {
		tempTable[ip] = id
	}
	for len(tempTable) > 0 {
		for ip, id := range tempTable {
			stream, err := node.NewStream(ctx, id, p2p.Protocol)
			if err != nil && (strings.HasPrefix(err.Error(), "failed to dial") ||
				strings.HasPrefix(err.Error(), "no addresses")) {
				// Attempt to connect to peers slowly when they aren't found.
				time.Sleep(5 * time.Second)
				continue
			}
			if err == nil {
				fmt.Printf("[+] Connection to %s Successful. Network Ready.\n", ip)
				stream.Close()
			}
			delete(tempTable, ip)
		}
	}
}

func verifyPort(port int) (int, error) {
	var ln net.Listener
	var err error

	// If a user manually sets a port don't try to automatically
	// find an open port.
	if port != 8001 {
		ln, err = net.Listen("tcp", ":"+strconv.Itoa(port))
		if err != nil {
			return port, errors.New("could not create node, listen port already in use by something else")
		}
	} else {
		// Automatically look for an open port when a custom port isn't
		// selected by a user.
		for {
			ln, err = net.Listen("tcp", ":"+strconv.Itoa(port))
			if err == nil {
				break
			}
			if port >= 65535 {
				return port, errors.New("failed to find open port")
			}
			port++
		}
	}
	if ln != nil {
		ln.Close()
	}
	return port, nil
}
