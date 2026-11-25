// Package fakeec2 is an in-process EC2 endpoint for tests: the real SDK
// client and the code under test run unchanged, and only the network is
// replaced by canned XML replies. Import it from _test.go files only.
package fakeec2

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/spf13/viper"
)

const ns = `xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"`

// Fake answers each EC2 Action with canned XML.
type Fake struct {
	mu      sync.Mutex
	replies map[string]string
	errs    map[string]string
	seq     map[string][]string
	calls   []string
	form    []map[string]string
}

// New starts the endpoint and returns it with a client for it. replies
// maps an Action to the XML inside its <Action>Response. The endpoint and
// dummy credentials are also exported through the environment, with
// aws.location set in viper, for code that builds its own client.
func New(t *testing.T, replies map[string]string) (*Fake, *ec2.Client) {
	t.Helper()
	f := &Fake{replies: map[string]string{}, errs: map[string]string{}, seq: map[string][]string{}}
	for k, v := range replies {
		f.replies[k] = v
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	t.Setenv("AWS_ENDPOINT_URL", srv.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
	viper.Set("aws.location", "us-east-1")
	t.Cleanup(func() { viper.Set("aws.location", nil) })

	return f, ec2.New(ec2.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(srv.URL),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
		HTTPClient:   srv.Client(),
		Retryer:      aws.NopRetryer{},
	})
}

// Reply sets the canned reply for action, replacing any earlier one and
// clearing a failure set by Fail.
func (f *Fake) Reply(action, xml string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies[action] = xml
	delete(f.seq, action)
	delete(f.errs, action)
}

// ReplySeq answers the first call to action with xmls[0], the next with
// xmls[1], and so on; the last reply repeats.
func (f *Fake) ReplySeq(action string, xmls ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq[action] = append([]string(nil), xmls...)
	delete(f.errs, action)
}

// Fail makes action answer with an API error of the given code.
func (f *Fake) Fail(action, code string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs[action] = code
}

// Called reports the Actions received, in order.
func (f *Fake) Called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// Field is the named form field of the n-th call.
func (f *Fake) Field(n int, name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.form[n][name]
}

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action := r.PostForm.Get("Action")
	fields := map[string]string{}
	for k := range r.PostForm {
		fields[k] = r.PostForm.Get(k)
	}

	f.mu.Lock()
	f.calls = append(f.calls, action)
	f.form = append(f.form, fields)
	code, failed := f.errs[action]
	body, ok := f.replies[action]
	if q := f.seq[action]; len(q) > 0 {
		body, ok = q[0], true
		if len(q) > 1 {
			f.seq[action] = q[1:]
		}
	}
	f.mu.Unlock()

	w.Header().Set("Content-Type", "text/xml")
	switch {
	case failed:
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<Response><Errors><Error><Code>%s</Code><Message>fake %s</Message></Error></Errors><RequestID>r</RequestID></Response>`, code, code)
	case !ok:
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<Response><Errors><Error><Code>UnknownAction</Code><Message>no canned reply for %s</Message></Error></Errors><RequestID>r</RequestID></Response>`, action)
	default:
		fmt.Fprintf(w, `<%sResponse %s><requestId>r</requestId>%s</%sResponse>`, action, ns, body, action)
	}
}
