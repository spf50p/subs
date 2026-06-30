package main

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Peer is a single VPN configuration entry from a subs.yaml file.
type Peer struct {
	Title      string `yaml:"title"`
	Comment    string `yaml:"comment"`
	Link       string `yaml:"link"`        // vpn://...
	ConfigFile string `yaml:"config_file"` // file name within work_dir/<uuid>/
	QR         *bool  `yaml:"qr,omitempty"` // whether to offer a QR code; nil means yes
}

// ShowQR reports whether the QR button should be rendered for this peer. QR is
// allowed unless the peer explicitly sets qr: false.
func (p Peer) ShowQR() bool {
	return p.QR == nil || *p.QR
}

// Client is the parsed content of a subs.yaml file: a flat list of peers.
type Client []Peer

// hasConfigFile reports whether any peer declares the given config file name.
func (c Client) hasConfigFile(name string) bool {
	for _, p := range c {
		if p.ConfigFile == name {
			return true
		}
	}
	return false
}

// qrAllowed reports whether QR generation is permitted for the peer that
// declares the given config file name. Unknown names return false.
func (c Client) qrAllowed(name string) bool {
	for _, p := range c {
		if p.ConfigFile == name {
			return p.ShowQR()
		}
	}
	return false
}

// loadClient reads and parses a subs.yaml file.
func loadClient(path string) (*Client, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Client
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &c, nil
}

// appendPeer adds a peer to the subs.yaml file at path, creating the file if
// it does not exist yet.
func appendPeer(path string, peer Peer) error {
	client, err := loadClient(path)
	if os.IsNotExist(err) {
		client = &Client{}
	} else if err != nil {
		return err
	}
	*client = append(*client, peer)
	return writeClient(path, *client)
}

// writeClient writes client as the entire contents of the subs.yaml file at
// path, replacing whatever was there before.
func writeClient(path string, client Client) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(client); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
