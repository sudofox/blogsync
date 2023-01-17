package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"strings"

	"github.com/motemen/go-wsse"
	"github.com/x-motemen/blogsync/atom"
)

type broker struct {
	*atom.Client
	*blogConfig
}

func newBroker(bc *blogConfig) *broker {
	return &broker{
		Client: &atom.Client{
			Client: &http.Client{
				Transport: &wsse.Transport{
					Username: bc.Username,
					Password: bc.Password,
				},
			},
		},
		blogConfig: bc,
	}
}

func (b *broker) FetchRemoteEntries() ([]*entry, error) {
	entries := []*entry{}
	//url := fmt.Sprintf("https://blog.hatena.ne.jp/%s/%s/atom/entry", b.Username, b.RemoteRoot)
	url := fmt.Sprintf("https://%s/feed", b.RemoteRoot)

	for {
		feed, err := b.Client.GetFeed(url)
		if err != nil {
			return nil, err
		}

		// if no entries, then break
		if len(feed.Entries) == 0 {
			break
		}


		for _, ae := range feed.Entries {
			e, err := entryFromAtom(&ae)
			if err != nil {
				return nil, err
			}

			entries = append(entries, e)
		}

		// get the oldest entry and extract the <published> attribute. Convert that into a unix timestamp to get the "next page" URL.
		oldestEntry := feed.Entries[len(feed.Entries)-1]
		// https://<b.remoteRoot>/feed?page=<unix timestamp>
		url = fmt.Sprintf("https://%s/feed?page=%d", b.RemoteRoot, oldestEntry.Published.Unix())
	}

	return entries, nil
}

func (b *broker) LocalPath(e *entry) string {
	extension := ".md" // TODO regard re.ContentType
	paths := []string{b.LocalRoot}
	if b.OmitDomain == nil || !*b.OmitDomain {
		paths = append(paths, strings.SplitN(b.RemoteRoot, "/", 2)[0])
	}
	paths = append(paths, e.URL.Path+extension)
	return filepath.Join(paths...)
}

func (b *broker) StoreFresh(e *entry, path string) (bool, error) {
	var localLastModified time.Time
	if fi, err := os.Stat(path); err == nil {
		localLastModified = fi.ModTime()
	}

	if e.LastModified.After(localLastModified) {
		logf("fresh", "remote=%s > local=%s", e.LastModified, localLastModified)
		if err := b.Store(e, path); err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

func (b *broker) Store(e *entry, path string) error {
	logf("store", "%s", path)

	dir, _ := filepath.Split(path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}

	_, err = f.WriteString(e.fullContent())
	if err != nil {
		return err
	}

	err = f.Close()
	if err != nil {
		return err
	}

	return os.Chtimes(path, *e.LastModified, *e.LastModified)
}

func (b *broker) UploadFresh(e *entry) (bool, error) {
	re, err := asEntry(b.Client.GetEntry(e.EditURL))
	if err != nil {
		return false, err
	}

	if e.LastModified.After(*re.LastModified) == false {
		return false, nil
	}

	return true, b.PutEntry(e)
}

func (b *broker) PutEntry(e *entry) error {
	newEntry, err := asEntry(b.Client.PutEntry(e.EditURL, e.atom()))
	if err != nil {
		return err
	}
	if e.CustomPath != "" {
		newEntry.CustomPath = e.CustomPath
	}

	path := b.LocalPath(newEntry)
	return b.Store(newEntry, path)
}

func (b *broker) PostEntry(e *entry) error {
	postURL := fmt.Sprintf("https://blog.hatena.ne.jp/%s/%s/atom/entry", b.Username, b.RemoteRoot)
	newEntry, err := asEntry(b.Client.PostEntry(postURL, e.atom()))
	if err != nil {
		return err
	}
	if e.CustomPath != "" {
		newEntry.CustomPath = e.CustomPath
	}

	path := b.LocalPath(newEntry)
	return b.Store(newEntry, path)
}
