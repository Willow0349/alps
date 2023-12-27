package alpscaldav

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/emersion/go-webdav/caldav"

	"git.sr.ht/~migadu/alps"
)

const (
	inputDateLayout = "2006-01-02"
	inputTimeLayout = "15:04"
)

func sanityCheckURL(u *url.URL) error {
	req, err := http.NewRequest(http.MethodOptions, u.String(), nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	// Servers might require authentication to perform an OPTIONS request
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("HTTP request failed: %v %v", resp.StatusCode, resp.Status)
	}
	return nil
}

func newPlugin(srv *alps.Server) (alps.Plugin, error) {
	u, err := srv.Upstream("caldavs", "caldav+insecure", "https", "http+insecure")
	if _, ok := err.(*alps.NoUpstreamError); ok {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("caldav: failed to parse upstream caldav server: %v", err)
	}
	switch u.Scheme {
	case "caldavs":
		u.Scheme = "https"
	case "caldav+insecure", "http+insecure":
		u.Scheme = "http"
	}
	if u.Scheme == "" {
		s, err := caldav.DiscoverContextURL(context.Background(), u.Host)
		if err != nil {
			srv.Logger().Printf("caldav: failed to discover CalDAV server: %v", err)
			return nil, nil
		}
		u, err = url.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("caldav: Discover returned an invalid URL: %v", err)
		}
	}

	if err := sanityCheckURL(u); err != nil {
		return nil, fmt.Errorf("caldav: failed to connect to CalDAV server %q: %v", u, err)
	}

	srv.Logger().Printf("Configured upstream CalDAV server: %v", u)

	p := alps.GoPlugin{Name: "caldav"}

	registerRoutes(&p, u)

	return p.Plugin(), nil
}

func init() {
	alps.RegisterPluginLoader(func(s *alps.Server) ([]alps.Plugin, error) {
		p, err := newPlugin(s)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, nil
		}
		return []alps.Plugin{p}, err
	})
}
