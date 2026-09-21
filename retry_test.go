package sys1

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestDefaultRetryPolicy(t *testing.T) {
	p := DefaultRetryPolicy()
	if p.MaxRetries != 2 {
		t.Errorf("MaxRetries = %d, want 2", p.MaxRetries)
	}
	if p.InitialBackoff != 500*time.Millisecond {
		t.Errorf("InitialBackoff = %v, want 500ms", p.InitialBackoff)
	}
	if p.MaxBackoff != 5*time.Second {
		t.Errorf("MaxBackoff = %v, want 5s", p.MaxBackoff)
	}
	if p.Jitter != 0.25 {
		t.Errorf("Jitter = %v, want 0.25", p.Jitter)
	}
	if p.IgnoreRetryAfter {
		t.Error("IgnoreRetryAfter = true, want false")
	}
	if p.MaxRetryAfter != 60*time.Second {
		t.Errorf("MaxRetryAfter = %v, want 60s", p.MaxRetryAfter)
	}
	if p.DisableConnRetries {
		t.Error("DisableConnRetries = true, want false")
	}
	if p.RetryStatuses != nil {
		t.Error("RetryStatuses is not nil, want nil (use default)")
	}
}

func TestRetryPolicyResolved(t *testing.T) {
	t.Run("zero durations take the defaults", func(t *testing.T) {
		got := RetryPolicy{MaxRetries: 5}.resolved()
		want := RetryPolicy{
			MaxRetries:     5,
			InitialBackoff: 500 * time.Millisecond,
			MaxBackoff:     5 * time.Second,
			MaxRetryAfter:  60 * time.Second,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("resolved() = %+v, want %+v", got, want)
		}
	})

	t.Run("set fields are kept", func(t *testing.T) {
		p := RetryPolicy{
			MaxRetries:         1,
			InitialBackoff:     time.Millisecond,
			MaxBackoff:         2 * time.Millisecond,
			Jitter:             0.5,
			IgnoreRetryAfter:   true,
			MaxRetryAfter:      3 * time.Millisecond,
			DisableConnRetries: true,
		}
		if got := p.resolved(); !reflect.DeepEqual(got, p) {
			t.Errorf("resolved() = %+v, want unchanged %+v", got, p)
		}
	})

	t.Run("default policy is a fixed point", func(t *testing.T) {
		p := DefaultRetryPolicy()
		if got := p.resolved(); !reflect.DeepEqual(got, p) {
			t.Errorf("resolved() = %+v, want unchanged %+v", got, p)
		}
	})
}

func TestDefaultRetryStatus(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{200, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{408, true},
		{422, false},
		{429, true},
		{499, false},
		{500, true},
		{502, true},
		{529, true},
		{599, true},
	}
	p := DefaultRetryPolicy()
	for _, tt := range tests {
		if got := p.shouldRetryStatus(tt.code); got != tt.want {
			t.Errorf("shouldRetryStatus(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestRetryPolicyCustomStatuses(t *testing.T) {
	p := RetryPolicy{RetryStatuses: func(code int) bool { return code == 599 }}
	if p.shouldRetryStatus(429) {
		t.Error("shouldRetryStatus(429) = true, want false with custom RetryStatuses")
	}
	if !p.shouldRetryStatus(599) {
		t.Error("shouldRetryStatus(599) = false, want true with custom RetryStatuses")
	}
}

func TestRetryPolicyValidate(t *testing.T) {
	tests := []struct {
		name    string
		p       RetryPolicy
		wantErr bool
	}{
		{"zero value", RetryPolicy{}, false},
		{"default", DefaultRetryPolicy(), false},
		{"negative max retries", RetryPolicy{MaxRetries: -1}, true},
		{"negative initial backoff", RetryPolicy{InitialBackoff: -1}, true},
		{"negative max backoff", RetryPolicy{MaxBackoff: -1}, true},
		{"jitter too low", RetryPolicy{Jitter: -0.1}, true},
		{"jitter too high", RetryPolicy{Jitter: 1.1}, true},
		{"jitter at bounds", RetryPolicy{Jitter: 1}, false},
		{"negative max retry after", RetryPolicy{MaxRetryAfter: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.validate()
			if tt.wantErr && err == nil {
				t.Error("validate() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validate() = %v, want nil", err)
			}
		})
	}
}

func TestRetryPolicyBackoffSchedule(t *testing.T) {
	p := RetryPolicy{
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		Jitter:         0, // deterministic
	}

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 500 * time.Millisecond},
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 5 * time.Second}, // capped at MaxBackoff
		{10, 5 * time.Second},
	}
	for _, tt := range tests {
		if got := p.backoff(tt.attempt); got != tt.want {
			t.Errorf("backoff(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestRetryPolicyBackoffJitterBounds(t *testing.T) {
	p := RetryPolicy{
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     1 * time.Second,
		Jitter:         0.25,
	}

	// Jitter of 0.25 means the delay is in [75%, 100%] of the
	// computed backoff.
	lo := 750 * time.Millisecond
	hi := 1 * time.Second
	for i := 0; i < 100; i++ {
		got := p.backoff(0)
		if got < lo || got > hi {
			t.Fatalf("backoff(0) = %v, want in [%v, %v]", got, lo, hi)
		}
	}
}

func TestRetryAfterParsing(t *testing.T) {
	tests := []struct {
		name    string
		header  http.Header
		wantOK  bool
		wantMin time.Duration
		wantMax time.Duration
	}{
		{
			name:   "no header",
			header: http.Header{},
			wantOK: false,
		},
		{
			name:    "seconds",
			header:  http.Header{"Retry-After": []string{"120"}},
			wantOK:  true,
			wantMin: 120 * time.Second,
			wantMax: 120 * time.Second,
		},
		{
			name:    "retry-after-ms",
			header:  http.Header{"Retry-After-Ms": []string{"1500"}},
			wantOK:  true,
			wantMin: 1500 * time.Millisecond,
			wantMax: 1500 * time.Millisecond,
		},
		{
			name: "retry-after-ms takes priority over Retry-After",
			header: http.Header{
				"Retry-After":    []string{"120"},
				"Retry-After-Ms": []string{"250"},
			},
			wantOK:  true,
			wantMin: 250 * time.Millisecond,
			wantMax: 250 * time.Millisecond,
		},
		{
			name:    "http date",
			header:  http.Header{"Retry-After": []string{time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)}},
			wantOK:  true,
			wantMin: 25 * time.Second,
			wantMax: 35 * time.Second,
		},
		{
			name:   "unparseable",
			header: http.Header{"Retry-After": []string{"not-a-value"}},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := retryAfter(tt.header)
			if ok != tt.wantOK {
				t.Fatalf("retryAfter() ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("retryAfter() = %v, want in [%v, %v]", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestRetryDelayRespectsMaxRetryAfter(t *testing.T) {
	p := RetryPolicy{
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		MaxRetryAfter:  60 * time.Second,
	}

	t.Run("honors Retry-After within bounds", func(t *testing.T) {
		h := http.Header{"Retry-After": []string{"10"}}
		got := retryDelay(p, h, 0)
		if got != 10*time.Second {
			t.Errorf("retryDelay = %v, want 10s", got)
		}
	})

	t.Run("falls back to backoff beyond MaxRetryAfter", func(t *testing.T) {
		h := http.Header{"Retry-After": []string{"3600"}} // 1 hour, over the 60s cap
		got := retryDelay(p, h, 0)
		if got > p.MaxBackoff {
			t.Errorf("retryDelay = %v, want <= MaxBackoff %v", got, p.MaxBackoff)
		}
	})

	t.Run("ignores Retry-After when IgnoreRetryAfter is set", func(t *testing.T) {
		p2 := p
		p2.IgnoreRetryAfter = true
		h := http.Header{"Retry-After": []string{"10"}}
		got := retryDelay(p2, h, 0)
		if got > p2.MaxBackoff {
			t.Errorf("retryDelay = %v, want <= MaxBackoff %v", got, p2.MaxBackoff)
		}
	})
}

func TestDefaultSleepFuncRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := defaultSleepFunc(ctx, 0); err == nil {
		t.Error("defaultSleepFunc with canceled ctx and 0 delay = nil, want error")
	}
	if err := defaultSleepFunc(ctx, time.Hour); err == nil {
		t.Error("defaultSleepFunc with canceled ctx = nil, want error")
	}
}
