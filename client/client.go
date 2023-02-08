package client

import (
	"context"
    "encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"net/http/cookiejar"
	"time"
	"io"
	"regexp"
	"strings"
	"os"
	"path"
)


type DownloadResourceResp struct {
	Err error
	Msg string
}

type Client struct {
	HTTPClient *http.Client
}

type ListBucketResult struct {
	XMLName     xml.Name `xml:"ListBucketResult"`
	Text        string   `xml:",chardata"`
	Xmlns       string   `xml:"xmlns,attr"`
	Name        string   `xml:"Name"`
	Prefix      string   `xml:"Prefix"`
	Marker      string   `xml:"Marker"`
	MaxKeys     string   `xml:"MaxKeys"`
	IsTruncated string   `xml:"IsTruncated"`
	Contents    []Content `xml:"Contents"`
} 

type Content struct {
	Text         string `xml:",chardata"`
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         string `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type ListBucketError struct {
	XMLName xml.Name `xml:"Error"`
	Text    string   `xml:",chardata"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
}

func (mw* Client) SearchBucket(ctx context.Context, bucketUrl string, query string, cookieUrl string, ignoreText string) ([]string, error) {
	cookies, err := retrieveCookies(cookieUrl)

	if err != nil {
		return nil, fmt.Errorf("There was an issue getting cookies for the bucket: %w", err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}

	mw.HTTPClient.Jar = jar

	hasMoreResults := true

	resources := make([]string, 0)

	for hasMoreResults {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, bucketUrl + query, nil)
		if err != nil {
			return nil, fmt.Errorf("could not create bucket search http request: %w", err)
		}

		jar.SetCookies(req.URL, cookies)
	
		resp, err := mw.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("could not fetch bucket search: %w", err)
		}
	
		defer resp.Body.Close()
	
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("could not fetch bucket search: status_code=%d url=%s", resp.StatusCode, req.URL)
		}
	
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("could not read response body: %w", err)
		}

		var bucketErr = new(ListBucketError)
		err = xml.Unmarshal(body, &bucketErr)
		if bucketErr.Code == "MissingKey" {
			return nil, fmt.Errorf("No cookie passed into request: %w", err)
		}

		var bucket ListBucketResult
		err = xml.Unmarshal(body, &bucket)
		if err != nil {
			return nil, fmt.Errorf("Issue unmarshalling xml into struct: %w", err)
		}

		contents := bucket.Contents

		if len(contents) == 0 {
			return nil, fmt.Errorf("There wasn't any resources found: %w", err)
		}

		for _, content := range contents {
			if !strings.Contains(content.Key, ignoreText) || ignoreText == "" {
				resources = append(resources, fmt.Sprintf("%s%s", bucketUrl, url.QueryEscape(content.Key)))
			}
		}

		re := regexp.MustCompile(`(?i)marker=(\w+)`)
		lastKeyOffset := contents[len(contents) - 1].Key
		query = re.ReplaceAllString(query, "marker=" + lastKeyOffset)
		// hasMoreResults, _ = strconv.ParseBool(bucket.IsTruncated)
		hasMoreResults = false
	}

	return resources, nil
}

func (mw* Client) DownloadResource(ctx context.Context, url string, cookieUrl string) (string, error) {
	fileName := path.Base(url)

	_, err := os.Stat("resources/" + fileName)
	if err == nil {
		// File exists already
		return url, err
	}


	cookies, err := retrieveCookies(cookieUrl) // make this a singleton

	if err != nil {
		return url, err
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}

	mw.HTTPClient.Jar = jar

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
		return url, err
    }

	jar.SetCookies(req.URL, cookies)

	resp, err := mw.HTTPClient.Do(req)
	if err != nil {
		return url, err
	}

	if resp.StatusCode != http.StatusOK {
		return url, err
	}

    defer resp.Body.Close()

	mkdirErr := os.MkdirAll("resources/" , os.ModePerm) 

	if mkdirErr != nil {
		// TODO: ignore?
    }

    file, err := os.Create("resources/" + fileName)
    if err != nil {
		return url, err
    }
    defer file.Close()

    _, err = io.Copy(file, resp.Body)
    if err != nil {
		return url, err
    }

	return url, nil
}

func retrieveCookies(cookieUrl string) ([]*http.Cookie, error) {
	if cookieUrl == "" {
		return nil, nil
	}
	c := &http.Client{Timeout: 10 * time.Second}
	res, err := c.Get(cookieUrl)

	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	return res.Cookies(), nil
}