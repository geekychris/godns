package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// Configuration options
type config struct {
	dnsServer  string
	dnsPort    int
	restServer string
	restPort   int
	timeout    time.Duration
	command    string
	hostname   string
	ips        []string
}

// EntryRequest represents a request to add or update a DNS entry
type EntryRequest struct {
	Hostname string   `json:"hostname"`
	IPs      []string `json:"ips"`
}

// EntryResponse represents a DNS entry response
type EntryResponse struct {
	Hostname string   `json:"hostname"`
	IPs      []string `json:"ips"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

func main() {
	cfg := parseFlags()

	// Execute the appropriate command
	var err error
	switch cfg.command {
	case "query":
		err = queryCommand(cfg)
	case "list":
		err = listCommand(cfg)
	case "get":
		err = getCommand(cfg)
	case "add":
		err = addCommand(cfg)
	case "delete":
		err = deleteCommand(cfg)
	default:
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() *config {
	cfg := &config{}
	
	// Define the command line flags
	flag.StringVar(&cfg.dnsServer, "dns-server", "localhost", "DNS server address")
	flag.IntVar(&cfg.dnsPort, "dns-port", 53, "DNS server port")
	flag.StringVar(&cfg.restServer, "rest-server", "localhost", "REST API server address")
	flag.IntVar(&cfg.restPort, "rest-port", 8080, "REST API server port")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Second, "Timeout for operations")
	
	// Custom usage function for better help output
	flag.Usage = printUsage
	
	// Parse the flags
	flag.Parse()
	
	// Get positional arguments
	args := flag.Args()
	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}
	
	// First positional argument is the command
	cfg.command = args[0]
	
	// Process remaining arguments based on the command
	switch cfg.command {
	case "query", "get", "delete":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: %s command requires a hostname\n", cfg.command)
			os.Exit(1)
		}
		cfg.hostname = args[1]
		
	case "add":
		if len(args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: add command requires a hostname and at least one IP address\n")
			os.Exit(1)
		}
		cfg.hostname = args[1]
		cfg.ips = args[2:]
		
	case "list":
		// No additional arguments needed
	
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", cfg.command)
		printUsage()
		os.Exit(1)
	}
	
	return cfg
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] command [arguments]\n\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  query <hostname>         Query DNS for a hostname")
	fmt.Fprintln(os.Stderr, "  list                     List all DNS entries via REST API")
	fmt.Fprintln(os.Stderr, "  get <hostname>           Get a specific DNS entry via REST API")
	fmt.Fprintln(os.Stderr, "  add <hostname> <ip> ...  Add or update a DNS entry via REST API")
	fmt.Fprintln(os.Stderr, "  delete <hostname>        Delete a DNS entry via REST API")
	fmt.Fprintln(os.Stderr, "\nOptions:")
	flag.PrintDefaults()
}

// DNS query command
func queryCommand(cfg *config) error {
	fmt.Printf("Querying DNS for hostname: %s\n", cfg.hostname)
	
	// Create a new DNS message
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(cfg.hostname), dns.TypeA)
	m.RecursionDesired = true
	
	// Create a DNS client
	c := new(dns.Client)
	c.Timeout = cfg.timeout
	
	// Send the query
	serverAddr := fmt.Sprintf("%s:%d", cfg.dnsServer, cfg.dnsPort)
	r, rtt, err := c.Exchange(m, serverAddr)
	if err != nil {
		return fmt.Errorf("DNS query failed: %v", err)
	}
	
	// Check for successful response
	if r.Rcode != dns.RcodeSuccess {
		return fmt.Errorf("DNS query returned error code: %d", r.Rcode)
	}
	
	// Display the results
	fmt.Printf("Query time: %v\n", rtt)
	fmt.Printf("Results for %s:\n", cfg.hostname)
	
	if len(r.Answer) == 0 {
		fmt.Println("No records found")
		return nil
	}
	
	for _, a := range r.Answer {
		if record, ok := a.(*dns.A); ok {
			fmt.Printf("  %s\n", record.A.String())
		}
	}
	
	return nil
}

// List all DNS entries
func listCommand(cfg *config) error {
	fmt.Println("Listing all DNS entries:")
	
	// Send GET request to REST API
	url := fmt.Sprintf("http://%s:%d/api/entries", cfg.restServer, cfg.restPort)
	resp, err := httpGet(url, cfg.timeout)
	if err != nil {
		return fmt.Errorf("failed to list entries: %v", err)
	}
	defer resp.Body.Close()
	
	// Read and parse the response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}
	
	// Check for error response
	if resp.StatusCode != http.StatusOK {
		return parseErrorResponse(body)
	}
	
	// Parse the entries
	var entries []EntryResponse
	if err := json.Unmarshal(body, &entries); err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}
	
	// Display the entries
	if len(entries) == 0 {
		fmt.Println("No entries found")
		return nil
	}
	
	for _, entry := range entries {
		fmt.Printf("Hostname: %s\n", entry.Hostname)
		fmt.Printf("  IP Addresses: %s\n", strings.Join(entry.IPs, ", "))
		fmt.Println()
	}
	
	return nil
}

// Get a specific DNS entry
func getCommand(cfg *config) error {
	fmt.Printf("Getting DNS entry for hostname: %s\n", cfg.hostname)
	
	// Send GET request to REST API
	url := fmt.Sprintf("http://%s:%d/api/entries/%s", cfg.restServer, cfg.restPort, cfg.hostname)
	resp, err := httpGet(url, cfg.timeout)
	if err != nil {
		return fmt.Errorf("failed to get entry: %v", err)
	}
	defer resp.Body.Close()
	
	// Read and parse the response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}
	
	// Check for error response
	if resp.StatusCode != http.StatusOK {
		return parseErrorResponse(body)
	}
	
	// Parse the entry
	var entry EntryResponse
	if err := json.Unmarshal(body, &entry); err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}
	
	// Display the entry
	fmt.Printf("Hostname: %s\n", entry.Hostname)
	fmt.Printf("IP Addresses: %s\n", strings.Join(entry.IPs, ", "))
	
	return nil
}

// Add or update a DNS entry
func addCommand(cfg *config) error {
	fmt.Printf("Adding/updating DNS entry for hostname: %s with IPs: %v\n", cfg.hostname, cfg.ips)
	
	// Validate IP addresses
	for _, ipStr := range cfg.ips {
		if net.ParseIP(ipStr) == nil {
			return fmt.Errorf("invalid IP address: %s", ipStr)
		}
	}
	
	// Create request payload
	entry := EntryRequest{
		Hostname: cfg.hostname,
		IPs:      cfg.ips,
	}
	
	// Marshal to JSON
	jsonData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	
	// Send PUT request to REST API
	url := fmt.Sprintf("http://%s:%d/api/entries", cfg.restServer, cfg.restPort)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{
		Timeout: cfg.timeout,
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()
	
	// Read and parse the response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}
	
	// Check for error response
	if resp.StatusCode != http.StatusOK {
		return parseErrorResponse(body)
	}
	
	fmt.Println("DNS entry added/updated successfully")
	return nil
}

// Delete a DNS entry
func deleteCommand(cfg *config) error {
	fmt.Printf("Deleting DNS entry for hostname: %s\n", cfg.hostname)
	
	// Send DELETE request to REST API
	url := fmt.Sprintf("http://%s:%d/api/entries/%s", cfg.restServer, cfg.restPort, cfg.hostname)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	
	client := &http.Client{
		Timeout: cfg.timeout,
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()
	
	// Check for success
	if resp.StatusCode == http.StatusNoContent {
		fmt.Println("DNS entry deleted successfully")
		return nil
	}
	
	// Read and parse error response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}
	
	return parseErrorResponse(body)
}

// Helper function for making GET requests
func httpGet(url string, timeout time.Duration) (*http.Response, error) {
	client := &http.Client{
		Timeout: timeout,
	}
	
	return client.Get(url)
}

// Helper function for parsing error responses
func parseErrorResponse(body []byte) error {
	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Errorf("server returned an error (failed to parse response)")
	}
	
	return fmt.Errorf("server returned an error: %s", errResp.Error)
}

