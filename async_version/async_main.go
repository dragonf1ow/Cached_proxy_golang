package main

import (
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Cache struct {
	m                 sync.RWMutex
	items             map[string]*Item
	defaultExpiration time.Duration
	cleanupInterval   time.Duration
}

type Item struct {
	StatusCode int
	Body       []byte
	Header     http.Header
	Created    time.Time
	Expiration int64
}

func NewCache(defaultExpiration, cleanupInterval time.Duration) *Cache {
	items := make(map[string]*Item)
	cache := &Cache{
		items:             items,
		defaultExpiration: defaultExpiration,
		cleanupInterval:   cleanupInterval,
	}

	return cache
}

func (c *Cache) AddItem(url string, statuscode int, header http.Header, body []byte, duration time.Duration) {

	var expiration int64

	if duration == 0 {

		duration = c.defaultExpiration
	}

	if duration > 0 {
		expiration = time.Now().Add(duration).UnixNano()
	}

	c.m.Lock()

	defer c.m.Unlock()

	log.Println(url)

	c.items[url] = &Item{
		StatusCode: statuscode,
		Body:       body,
		Header:     header,
		Created:    time.Now(),
		Expiration: expiration,
	}
}

func (c *Cache) CheckValue(url string) bool {
	for key, _ := range c.items {
		if key == url {
			return true
		}
	}

	return false
}

func (c *Cache) GetItem(url string) (*Item, bool) {
	item, found := c.items[url]

	if !found {
		return nil, false
	}
	if item.Expiration > 0 && time.Now().UnixNano() > item.Expiration {
		return nil, false
	}

	return item, true
}

func ProxyHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	switch path {
	case "":
		w.Write([]byte("Usage: http://localhost:8080/avito.ru"))
		return
	}

	if r.URL.Path == "/favicon.ico" {
		http.NotFound(w, r)
		log.Println("Page favicon.ico not found!")
	}

	var targetURL string
	if strings.Contains(path, "://") {
		targetURL = path
	} else {
		targetURL = "https://" + path
	}

	if r.URL.RawQuery != "" {
		if strings.Contains(targetURL, "?") {
			targetURL += "&" + r.URL.RawQuery
		} else {
			targetURL += "?" + r.URL.RawQuery
		}
	}

	if item, found := cache.GetItem(targetURL); found {
		for key, values := range item.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(item.StatusCode)

		w.Write(item.Body)
		log.Println("Page from Cache")
		return

	} else {

		client := &http.Client{
			Timeout: 30 * time.Second,
		}
		go func() {

			resp, err := client.Get(targetURL)
			if err != nil {
				log.Println(err)

				w.Write([]byte("Ошибка при запросе"))
				return
			}

			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)

			go cache.AddItem(targetURL, resp.StatusCode, resp.Header, body, 0)

			for key, values := range resp.Header {
				for _, value := range values {
					w.Header().Add(key, value)
				}

			}
			w.WriteHeader(resp.StatusCode)

			w.Write(body)
		}()
	}

}

var cache = NewCache(5*time.Minute, 10*time.Minute)

func main() {
	go http.HandleFunc("/", ProxyHandler)

	server := &http.Server{
		Addr:    ":8080",
		Handler: nil,
	}

	err := server.ListenAndServe()
	if err != nil {
		log.Println(err)
	}

}
