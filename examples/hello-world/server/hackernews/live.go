//go:build !js

package hackernews

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	topStories = "https://hacker-news.firebaseio.com/v0/topstories.json"
	itemFormat = "https://hacker-news.firebaseio.com/v0/item/%d.json"
	wanted     = 5
	budget     = 2 * time.Second
)

func Top(request *http.Request) ([]Story, bool) {
	client := &http.Client{Timeout: budget}
	var ids []int
	if err := read(request, client, topStories, &ids); err != nil {
		return offline, false
	}
	stories := make([]Story, 0, wanted)
	for _, id := range ids {
		if len(stories) == wanted {
			break
		}
		var story Story
		if err := read(request, client, fmt.Sprintf(itemFormat, id), &story); err != nil {
			break
		}
		if story.Title == "" {
			continue
		}
		if story.URL == "" {
			story.URL = fmt.Sprintf("https://news.ycombinator.com/item?id=%d", id)
		}
		stories = append(stories, story)
	}
	if len(stories) == 0 {
		return offline, false
	}
	return stories, true
}

func read(request *http.Request, client *http.Client, url string, target any) error {
	outbound, err := http.NewRequestWithContext(request.Context(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(outbound)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %d", url, response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}
