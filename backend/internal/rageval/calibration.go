package rageval

import (
	"math"
	"sort"

	"cortex/backend/internal/store"
)

type CalibrationPoint struct {
	Threshold             float64 `json:"threshold"`
	AnswerableRecall      float64 `json:"answerable_recall"`
	UnanswerableRejection float64 `json:"unanswerable_rejection"`
	FalseAnswerRate       float64 `json:"false_answer_rate"`
}

type Calibration struct {
	RecommendedThreshold float64            `json:"recommended_threshold"`
	Points               []CalibrationPoint `json:"points"`
	AnswerableCases      int                `json:"answerable_cases"`
	UnanswerableCases    int                `json:"unanswerable_cases"`
}

func CalibrateRerankThreshold(cases []Case, results []Result, minimumAnswerableRecall float64) Calibration {
	thresholds := []float64{}
	for _, result := range results {
		if len(result.AfterRerank) > 0 && result.AfterRerank[0].RerankScore != nil {
			thresholds = append(thresholds, *result.AfterRerank[0].RerankScore)
		}
	}
	sort.Float64s(thresholds)
	thresholds = uniqueFloats(thresholds)
	thresholds = midpointThresholds(thresholds)
	calibration := Calibration{}
	for _, c := range cases {
		if isAnswerable(c) {
			calibration.AnswerableCases++
		} else {
			calibration.UnanswerableCases++
		}
	}
	for _, threshold := range thresholds {
		var answered, rejected, falseAnswers int
		for i, c := range cases {
			accepted := i < len(results) && topScore(results[i]) >= threshold
			if isAnswerable(c) {
				if accepted && goldAboveThreshold(c.SourcePaths, results[i], threshold) {
					answered++
				}
			} else if accepted {
				falseAnswers++
			} else {
				rejected++
			}
		}
		calibration.Points = append(calibration.Points, CalibrationPoint{Threshold: threshold,
			AnswerableRecall: ratio(answered, calibration.AnswerableCases), UnanswerableRejection: ratio(rejected, calibration.UnanswerableCases), FalseAnswerRate: ratio(falseAnswers, calibration.UnanswerableCases)})
	}
	best := -1
	for i, point := range calibration.Points {
		if point.AnswerableRecall < minimumAnswerableRecall {
			continue
		}
		if best < 0 || point.FalseAnswerRate < calibration.Points[best].FalseAnswerRate ||
			(point.FalseAnswerRate == calibration.Points[best].FalseAnswerRate && point.AnswerableRecall > calibration.Points[best].AnswerableRecall) ||
			(point.FalseAnswerRate == calibration.Points[best].FalseAnswerRate && point.AnswerableRecall == calibration.Points[best].AnswerableRecall && point.Threshold > calibration.Points[best].Threshold) {
			best = i
		}
	}
	if best < 0 && len(calibration.Points) > 0 {
		best = 0
		for i, point := range calibration.Points {
			if point.AnswerableRecall > calibration.Points[best].AnswerableRecall {
				best = i
			}
		}
	}
	if best >= 0 {
		calibration.RecommendedThreshold = calibration.Points[best].Threshold
	}
	return calibration
}

func isAnswerable(c Case) bool { return c.Answerable == nil || *c.Answerable }
func topScore(r Result) float64 {
	if len(r.AfterRerank) == 0 || r.AfterRerank[0].RerankScore == nil {
		return math.Inf(-1)
	}
	return *r.AfterRerank[0].RerankScore
}
func goldAboveThreshold(gold []string, r Result, threshold float64) bool {
	for _, item := range r.AfterRerank {
		if item.RerankScore != nil && *item.RerankScore >= threshold && firstRank(gold, []store.KnowledgeCandidate{{Title: item.Title}}, 1) > 0 {
			return true
		}
	}
	return false
}
func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}
func uniqueFloats(in []float64) []float64 {
	out := []float64{}
	for _, v := range in {
		if len(out) == 0 || v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}

func midpointThresholds(scores []float64) []float64 {
	if len(scores) < 2 {
		return scores
	}
	thresholds := make([]float64, 0, len(scores)-1)
	for i := 0; i+1 < len(scores); i++ {
		thresholds = append(thresholds, scores[i]+(scores[i+1]-scores[i])/2)
	}
	return thresholds
}
