package transmission

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

const (
	// DefaultAddress default transmission address
	DefaultAddress = "http://localhost:9091/transmission/rpc"
)

// Config used to configure transmission client
type Config struct {
	// Address defaultt http://localhost:9091/transmission/rpc
	Address  string
	User     string
	Password string
	// SkipCheckSSL set to true if you use untrusted certificat default false
	SkipCheckSSL bool
}

// Client transmission client
type Client struct {
	httpClient *http.Client
	conf       *Config
	sessionID  string
	endpoint   string
}

type getTorrentArg struct {
	Fields []string `json:"fields,omitempty"`
	Ids    []int    `json:"ids,omitempty"`
}

type addTorrentArg struct {
	// Cookies string
	// download-dir string
	// Filename filename or URL of the .torrent file
	Filename string `json:"filename,omitempty"`
	// Metainfo base64-encoded .torrent content
	Metainfo string `json:"metainfo,omitempty"`
	// Paused   bool
	// peer-limit int
	// BandwidthPriority int
	// files-wanted
	// files-unwanted
	// priority-high
	// priority-low
	// priority-normal

}

type removeTorrentArg struct {
	Ids             []int `json:"ids,string"`
	DeleteLocalData bool  `json:"delete-local-data,omitempty"`
}

// Request object for API call
type Request struct {
	Method    string      `json:"method"`
	Arguments interface{} `json:"arguments"`
}

// Response object for API cal response
type Response struct {
	Arguments interface{} `json:"arguments"`
	Result    string      `json:"result"`
}

// Do low level function for interact with transmission only take care
// of authentification and session id
func (c *Client) Do(req *http.Request, retry bool) (*http.Response, error) {
	if c.conf.User != "" && c.conf.Password != "" {
		req.SetBasicAuth(c.conf.User, c.conf.Password)
	}
	if c.sessionID != "" {
		req.Header.Add("X-Transmission-Session-Id", c.sessionID)
	}

	//Body copy for replay it if needed
	b, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = ioutil.NopCloser(bytes.NewBuffer(b))

	//Log request for debug
	log.Print(bytes.NewBuffer(b).String())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	// error 409
	if resp.StatusCode == http.StatusConflict && retry {
		c.sessionID = resp.Header.Get("X-Transmission-Session-Id")
		req.Body = ioutil.NopCloser(bytes.NewBuffer(b))
		return c.Do(req, false)
	}
	return resp, nil
}

func (c *Client) post(tReq *Request) (*http.Response, error) {
	data, err := json.Marshal(tReq)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", c.endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return c.Do(req, true)
}

func (c *Client) request(tReq *Request, tResp *Response) error {
	resp, err := c.post(tReq)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, tResp)
	if err != nil {
		return err
	}
	if tResp.Result != "success" {
		return fmt.Errorf("transmission: request response %q", tResp.Result)
	}
	return nil
}

// GetTorrents return list of torrent
func (c *Client) GetTorrents() (*[]Torrent, error) {
	tReq := &Request{
		Arguments: getTorrentArg{
			Fields: torrentGetFields,
		},
		Method: "torrent-get",
	}

	r := &Response{Arguments: &Torrents{}}

	err := c.request(tReq, r)
	if err != nil {
		return nil, err
	}

	t := *r.Arguments.(*Torrents).Torrents
	for i := 0; i < len(t); i++ {
		t[i].Client = c
	}
	return &t, nil
}

// AddTorrent add torrent from filename or metadata
// filename is an url or a path
// metadata is base64 encoded content of torrent file
func (c *Client) AddTorrent(filename, metadata string) (*Torrent, error) {
	tReq := &Request{
		Arguments: addTorrentArg{
			Filename: filename,
			Metainfo: metadata,
		},
		Method: "torrent-add",
	}
	type added struct {
		Torrent *Torrent `json:"torrent-added"`
	}
	r := &Response{Arguments: &added{}}
	err := c.request(tReq, r)
	if err != nil {
		return nil, err
	}
	t := r.Arguments.(*added)
	t.Torrent.Client = c
	return t.Torrent, nil
}

// RemoveTorrents remove torrents
func (c *Client) RemoveTorrents(torrents []*Torrent, removeData bool) error {
	ids := make([]int, len(torrents))
	for i := range torrents {
		ids[i] = torrents[i].ID
	}
	tReq := &Request{
		Arguments: removeTorrentArg{
			Ids:             ids,
			DeleteLocalData: removeData,
		},
		Method: "torrent-remove",
	}
	r := &Response{}
	err := c.request(tReq, r)
	if err != nil {
		return err
	}
	return nil
}

// New create a new transmission client
func New(conf Config) (*Client, error) {
	httpClient := &http.Client{}
	if conf.SkipCheckSSL {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		httpClient = &http.Client{Transport: tr}
	}
	if conf.Address == "" {
		conf.Address = DefaultAddress
	}
	return &Client{conf: &conf, httpClient: httpClient, endpoint: conf.Address}, nil
}
