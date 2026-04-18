package app

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

type profileRiskAPIResponse struct {
	ProfileKind             string  `json:"profile_kind"`
	ProfileName             string  `json:"profile_name"`
	ProfileKey              string  `json:"profile_key"`
	LastProfileIP           string  `json:"last_profile_ip,omitempty"`
	LastMatchedDomain       string  `json:"last_matched_domain,omitempty"`
	LastMatchedReason       string  `json:"last_matched_reason,omitempty"`
	LastAction              string  `json:"last_action,omitempty"`
	LastBlockAt             string  `json:"last_block_at,omitempty"`
	PendingBlockAt          string  `json:"pending_block_at,omitempty"`
	Score15m                int     `json:"score_15m"`
	Score24h                int     `json:"score_24h"`
	EffectiveScore          int     `json:"effective_score"`
	NotifyThreshold         int     `json:"notify_threshold"`
	BlockThreshold15m       int     `json:"block_threshold_15m"`
	BlockThreshold24h       int     `json:"block_threshold_24h"`
	PercentToNotify         float64 `json:"percent_to_notify"`
	PercentToBlock15m       float64 `json:"percent_to_block_15m"`
	PercentToBlock24h       float64 `json:"percent_to_block_24h"`
	PercentToCritical       float64 `json:"percent_to_critical"`
	CriticalWindow          string  `json:"critical_window"`
	CriticalThreshold       int     `json:"critical_threshold"`
	CriticalThresholdReached bool   `json:"critical_threshold_reached"`
	GeneratedAt             string  `json:"generated_at"`
}

func startAPIServer(cfg Config) {
	addr := strings.TrimSpace(cfg.HTTPListenAddr)
	token := strings.TrimSpace(cfg.HTTPAPIToken)
	if addr == "" {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/profile-risk", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if token != "" && !matchBearerToken(r.Header.Get("Authorization"), token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if kind == "" || name == "" {
			http.Error(w, "kind and name are required", http.StatusBadRequest)
			return
		}
		resp, ok, err := buildProfileRiskResponse(cfg, kind, name, time.Now().UTC())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	go func() {
		log.Printf("dns-guard api listening on %s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("dns-guard api stopped: %v", err)
		}
	}()
}

func matchBearerToken(headerValue, expected string) bool {
	headerValue = strings.TrimSpace(headerValue)
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	if !strings.HasPrefix(strings.ToLower(headerValue), "bearer ") {
		return false
	}
	return strings.TrimSpace(headerValue[len("Bearer "):]) == expected
}

func buildProfileRiskResponse(cfg Config, kind, name string, now time.Time) (profileRiskAPIResponse, bool, error) {
	state, err := loadState(cfg.StatePath)
	if err != nil {
		return profileRiskAPIResponse{}, false, err
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	name = strings.TrimSpace(name)
	profileKey := kind + ":" + strings.ToLower(name)
	ps, ok := state.Profiles[profileKey]
	if !ok {
		exists, err := profileExists(cfg, kind, name)
		if err != nil {
			return profileRiskAPIResponse{}, false, err
		}
		if !exists {
			return profileRiskAPIResponse{}, false, nil
		}
		ps = ProfileRiskState{Buckets: map[string]int{}}
	}
	score15m, score24h := calculateScores(ps, now.UTC())
	effectiveScore := score15m
	criticalWindow := "15m"
	criticalThreshold := cfg.ScoreBlockAt15m
	percentCritical := percentOfThreshold(score15m, cfg.ScoreBlockAt15m)
	if score24h > effectiveScore {
		effectiveScore = score24h
	}
	percent24h := percentOfThreshold(score24h, cfg.ScoreBlockAt24h)
	if percent24h >= percentCritical {
		percentCritical = percent24h
		criticalWindow = "24h"
		criticalThreshold = cfg.ScoreBlockAt24h
	}
	return profileRiskAPIResponse{
		ProfileKind:              kind,
		ProfileName:              name,
		ProfileKey:               profileKey,
		LastProfileIP:            ps.LastProfileIP,
		LastMatchedDomain:        ps.LastMatchedDomain,
		LastMatchedReason:        ps.LastMatchedReason,
		LastAction:               ps.LastAction,
		LastBlockAt:              ps.LastBlockAt,
		PendingBlockAt:           ps.PendingBlockAt,
		Score15m:                 score15m,
		Score24h:                 score24h,
		EffectiveScore:           effectiveScore,
		NotifyThreshold:          cfg.ScoreNotifyAt,
		BlockThreshold15m:        cfg.ScoreBlockAt15m,
		BlockThreshold24h:        cfg.ScoreBlockAt24h,
		PercentToNotify:          percentOfThreshold(effectiveScore, cfg.ScoreNotifyAt),
		PercentToBlock15m:        percentOfThreshold(score15m, cfg.ScoreBlockAt15m),
		PercentToBlock24h:        percentOfThreshold(score24h, cfg.ScoreBlockAt24h),
		PercentToCritical:        percentCritical,
		CriticalWindow:           criticalWindow,
		CriticalThreshold:        criticalThreshold,
		CriticalThresholdReached: criticalThreshold > 0 && percentCritical >= 100,
		GeneratedAt:              now.UTC().Format(time.RFC3339),
	}, true, nil
}

func profileExists(cfg Config, kind, name string) (bool, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	want := strings.ToLower(strings.TrimSpace(name))
	if kind == "" || want == "" {
		return false, nil
	}
	switch kind {
	case "wg":
		if !cfg.WG.Enabled {
			return false, nil
		}
		peers, err := listWGPeers(cfg.WG)
		if err != nil {
			return false, err
		}
		for _, peer := range peers {
			if strings.ToLower(strings.TrimSpace(peer.Name)) == want {
				return true, nil
			}
		}
		return false, nil
	case "awg":
		if !cfg.AWG.Enabled {
			return false, nil
		}
		peers, err := listWGPeers(cfg.AWG)
		if err != nil {
			return false, err
		}
		for _, peer := range peers {
			if strings.ToLower(strings.TrimSpace(peer.Name)) == want {
				return true, nil
			}
		}
		return false, nil
	case "ovpn":
		if !cfg.OVPN.Enabled {
			return false, nil
		}
		profiles, err := listOVPNCertificates(cfg.OVPN)
		if err != nil {
			return false, err
		}
		for _, profile := range profiles {
			if strings.ToLower(strings.TrimSpace(profile.Name)) == want {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, nil
	}
}

func percentOfThreshold(score, threshold int) float64 {
	if threshold <= 0 {
		return 0
	}
	raw := (float64(score) * 1000) / float64(threshold)
	return float64(int(raw+0.5)) / 10
}
