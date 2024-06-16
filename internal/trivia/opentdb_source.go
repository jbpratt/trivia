// Package trivia ...
package trivia

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const openTDBBaseURL = "https://opentdb.com/"

type OpenTDBSource struct {
	client    *http.Client
	token     string
	cacheSize int
	cache     []*Question
}

func NewDefaultOpenTDBSource() (*OpenTDBSource, error) {
	return NewOpenTDBSource(15)
}

func NewOpenTDBSource(cacheSize int) (*OpenTDBSource, error) {
	client := &http.Client{}
	resp, err := client.Get(fmt.Sprintf("%s/api_token.php?command=request", openTDBBaseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to request token: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned invalid http code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response body: %w", err)
	}

	tokenRes := struct {
		Token string `json:"token"`
	}{}

	if err = json.Unmarshal(body, &tokenRes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	s := &OpenTDBSource{
		client:    client,
		token:     tokenRes.Token,
		cacheSize: cacheSize,
	}
	return s, nil
}

func (s *OpenTDBSource) refreshCache() error {
	u, err := url.Parse(fmt.Sprintf("%s/api.php", openTDBBaseURL))
	if err != nil {
		return fmt.Errorf("failed to parse url: %w", err)
	}

	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return fmt.Errorf("failed to parse query: %w", err)
	}

	q.Add("token", s.token)
	q.Add("amount", fmt.Sprint(s.cacheSize))
	u.RawQuery = q.Encode()

	resp, err := s.client.Get(u.String())
	if err != nil {
		return fmt.Errorf("failed to get api data: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned bad http status: %d", resp.StatusCode)
	}

	var resultsResp struct {
		ResponseCode int `json:"response_code"`
		Results      []struct {
			Type             string   `json:"type"`
			Question         string   `json:"question"`
			CorrectAnswer    string   `json:"correct_answer"`
			IncorrectAnswers []string `json:"incorrect_answers"`
		} `json:"results"`
	}

	if err = json.NewDecoder(resp.Body).Decode(&resultsResp); err != nil {
		return fmt.Errorf("failed to unmarshal api response body: %w", err)
	}

	if len(resultsResp.Results) == 0 {
		return fmt.Errorf("server returned no results: %v", resultsResp)
	}

	for _, result := range resultsResp.Results {
		q := &Question{
			Question: result.Question,
			Type:     result.Type,
			Answers: []*Answer{
				{result.CorrectAnswer, true},
			},
		}

		for _, value := range result.IncorrectAnswers {
			q.Answers = append(q.Answers, &Answer{value, false})
		}

		s.cache = append(s.cache, q)
	}
	return nil
}

func (s *OpenTDBSource) Question() (*Question, error) {
	if len(s.cache) == 0 {
		if err := s.refreshCache(); err != nil {
			return nil, err
		}
	}

	l := len(s.cache) - 1
	q := s.cache[l]
	s.cache = s.cache[:l]

	return q, nil
}
