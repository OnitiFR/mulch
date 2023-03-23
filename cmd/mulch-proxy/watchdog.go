package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

/*
	This module is here because we've had issues with proxy-chaining when
	the child-parent link is using HTTP2 (ie: child with HTTPS
	proxy_chain_child_url setting).

	A watchdog is now set (the parent watches its child) in this exact situation.

	After a few times (ex: 10 days) the multiplexed HTTP2 connection between
	the parent and one child may hangs, as the parent became unable to
	read child responses. The parent is then unable to serve ANY request for
	this child (as all requests are multiplexed in the same HTTP2 link).

	This is caused by deadlock in x/net/http2, see related golang issues:
	* https://github.com/golang/go/issues/32388
	* https://github.com/golang/go/issues/33425
	* https://github.com/golang/go/issues/39812

	The x/net/http2 API does not provides access to the underlying connections,
	so we have no clean way to kill the faulty HTTP2 link.

	The best (current) way for us to mitigate this situation is to detect
	this deadlock ("child is responding [TCP] but not with HTTP2") and kill
	the whole proccess. Systemd (or whatever service manager) will instantly
	restart the proxy, causing a short downtime every two weeks or so. Better
	than a full lock of a child :(

	Since then a new symptom appeared, where the front itself is fully locked.
	(dump is full of Lock() calls). We extend the watchdog to also check
	the front itself.
*/

// dialTest will attempt a TCP connection to host in the urlIn,
// with default HTTP/HTTPS port number if not provided in the URL
// returns nil error if connection was successful
func dialTest(urlIn string, timeout time.Duration) error {
	urlObj, err := url.Parse(urlIn)
	if err != nil {
		return err
	}
	host := ""
	port := ""

	if urlObj.Port() != "" {
		// port is explicit
		host, port, err = net.SplitHostPort(urlObj.Host)
		if err != nil {
			return err
		}
	} else {
		host = urlObj.Host
		port = "80"
		if urlObj.Scheme == ProtoHTTPS {
			port = "443"
		}
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return err
	}
	if conn != nil {
		conn.Close()
		return nil
	}
	return errors.New("no error, but no connection either")
}

func watchProxy(url string, log *Log) error {
	// check if we get a TCP connection to this proxy
	err := dialTest(url, 5*time.Second)
	if err != nil {
		log.Errorf("watchdog: proxy %s seems down (%s)", url, err)
		return nil
	}

	// -- proxy seems up, so let's see if we can get an HTTP2 response
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Errorf("watchdog: %s", err.Error())
		return nil
	}
	req.Header.Set(WatchDogHeaderName, "true")

	http2Client := &http.Client{
		// create our own connection
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
		// was 5, but we've seen some timeouts (up to a few times per day), let's investigate
		Timeout: time.Duration(10 * time.Second),
	}

	start := time.Now()
	response2, err := http2Client.Do(req)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() { // is timeout?
			// OK, we have a REAL problem with this proxy, here!
			log.Infof("watchdog: HTTP2 timeout (%s) for %s", time.Since(start), url)
			return err
		}
		log.Errorf("watchdog: HTTP2: %s (%s)", err.Error(), url) // another error
		return nil
	}
	defer response2.Body.Close()

	// (might remove this trace in the future, it's here to investigate 5s timeouts)
	log.Tracef("watchdog: HTTP2 response for %s in %s", url, time.Since(start))

	// drain response
	_, err = io.ReadAll(response2.Body)
	if err != nil {
		// we've also seen timeout related issues* thru this drain, so we consider this as a fatal failure
		// * "context deadline exceeded (Client.Timeout or context cancellation while reading body)"
		return fmt.Errorf("HTTP2 drain: %s", err.Error())
	}

	// everything is OK for this proxy
	return nil
}

func watchProxies(ddb *DomainDatabase, selfUrl url.URL, log *Log) {
	// find all children proxies configured with HTTPS
	children := ddb.GetChildren()

	log.Tracef("watchdog: checking children (%d) + ourself", len(children))

	for _, childURL := range children {
		urlObj, _ := url.Parse(childURL)
		if urlObj.Scheme == ProtoHTTPS {
			err := watchProxy(childURL, log)
			if err != nil {
				log.Errorf("watchdog: %s", err.Error())
				log.Errorf("FATAL: watchdog unable to contact child using HTTP2 while child seems up, possible chain deadlock! Exiting process for a forced restart.")
				os.Exit(200)
			}
		}
	}

	if selfUrl.Scheme == ProtoHTTPS {
		// remove port from selfUrl (we want to contact our proxy server, not the API)
		selfUrl.Host = selfUrl.Hostname()

		err := watchProxy(selfUrl.String(), log)
		if err != nil {
			log.Error(err.Error())
			log.Errorf("FATAL: watchdog unable to contact ourself using HTTP2, possible deadlock! Exiting process for a forced restart.")
			os.Exit(200)
		}
	}

	log.Trace("watchdog: end")
}

// InstallWatchdog will check HTTP2 links to our children (as a parent proxy)
// and to ourself (we may increase frequency later)
func InstallWatchdog(ddb *DomainDatabase, selfUrl *url.URL, log *Log) {
	go func() {
		for {
			time.Sleep(30 * time.Second)
			watchProxies(ddb, *selfUrl, log)
		}
	}()
}
