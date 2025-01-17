package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/nbcnews/graphql/internal/jsonutil"
)

// ClientOptFunc graphql client option
type ClientOptFunc func(*Client)

// WithHeader set graphql client header
func WithHeader(key, val string) ClientOptFunc {
	return func(c *Client) {
		c.headers[key] = val
	}
}

// WithCookie set graphql client cookie
func WithCookie(key, val string) ClientOptFunc {
	return func(c *Client) {
		if c.cookies == nil {
			c.cookies = map[string]string{}
		}
		c.cookies[key] = val
	}
}

// Client is a GraphQL client.
type Client struct {
	url        string // GraphQL server URL.
	httpClient *http.Client
	headers,
	cookies map[string]string
}

// NewClient creates a GraphQL client targeting the specified GraphQL server URL.
// If httpClient is nil, then http.DefaultClient is used.
func NewClient(url string, httpClient *http.Client, opts ...ClientOptFunc) (c *Client) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	c = &Client{
		headers: 	map[string]string{
			"Content-Type": "application/json",
		},
		url:        url,
		httpClient: httpClient,
	}
	for _, optf := range opts {
		optf(c)
	}
	return c
}

// Query executes a single GraphQL query request,
// with a query derived from q, populating the response into it.
// q should be a pointer to struct that corresponds to the GraphQL schema.
func (c *Client) Query(ctx context.Context, q interface{}, variables map[string]interface{}, operationName string) error {
	return c.do(ctx, queryOperation, q, variables, operationName)
}

// Mutate executes a single GraphQL mutation request,
// with a mutation derived from m, populating the response into it.
// m should be a pointer to struct that corresponds to the GraphQL schema.
func (c *Client) Mutate(ctx context.Context, m interface{}, variables map[string]interface{}, operationName string) error {
	return c.do(ctx, mutationOperation, m, variables, operationName)
}

// do executes a single GraphQL operation.
func (c *Client) do(ctx context.Context, op operationType, v interface{}, variables map[string]interface{}, operationName string) error {
	var query string
	switch op {
	case queryOperation:
		query = constructQuery(v, variables, operationName)
	case mutationOperation:
		query = constructMutation(v, variables)
	}
	in := struct {
		Query         string                 `json:"query"`
		OperationName string                 `json:"operationName"`
		Variables     map[string]interface{} `json:"variables,omitempty"`
	}{
		OperationName: operationName,
		Query:         query,
		Variables:     variables,
	}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(in)
	if err != nil {
		return err
	}
	var (
		req  *http.Request
		resp *http.Response
	)
	// fmt.Println(buf.String())
	if req, err = http.NewRequest("POST", c.url, &buf); err != nil {
		return err
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	for k, v := range c.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	if resp, err = c.httpClient.Do(req.WithContext(ctx)); err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("non-200 OK status code: %v body: %q", resp.Status, body)
	}
	var out struct {
		Data   *json.RawMessage
		Errors errors
		//Extensions interface{} // Unused.
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		// TODO: Consider including response body in returned error, if deemed helpful.
		return err
	}
	if out.Data != nil {
		err := jsonutil.UnmarshalGraphQL(*out.Data, v)
		if err != nil {
			// TODO: Consider including response body in returned error, if deemed helpful.
			return err
		}
	}
	if len(out.Errors) > 0 {
		return out.Errors
	}
	return nil
}

// errors represents the "errors" array in a response from a GraphQL server.
// If returned via error interface, the slice is expected to contain at least 1 element.
//
// Specification: https://facebook.github.io/graphql/#sec-Errors.
type errors []struct {
	Message   string
	Locations []struct {
		Line   int
		Column int
	}
}

// Error implements error interface.
func (e errors) Error() string {
	return e[0].Message
}

type operationType uint8

const (
	queryOperation operationType = iota
	mutationOperation
	//subscriptionOperation // Unused.
)
