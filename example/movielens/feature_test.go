package movielens

import (
	"context"
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	. "github.com/smartystreets/goconvey/convey"
	"gonum.org/v1/gonum/mat"

	"github.com/auxten/edgeRec/nn/metrics"
	nn "github.com/auxten/edgeRec/nn/neural_network"
	rcmd "github.com/auxten/edgeRec/recommend"
)

func TestFeatureEngineer(t *testing.T) {
	var (
		recSys = &RecSysImpl{
			DataPath:  "movielens.db",
			SampleCnt: 79948,
		}
		model rcmd.Predictor
		err   error
	)
	fiter := nn.NewMLPClassifier(
		[]int{100},
		"relu", "adam", 1e-5,
	)
	fiter.Verbose = true
	fiter.MaxIter = 100
	fiter.LearningRate = "adaptive"
	fiter.LearningRateInit = .0025

	trainCtx := context.Background()
	log.SetLevel(log.DebugLevel)
	Convey("feature engineering", t, func() {
		model, err = rcmd.Train(trainCtx, recSys, fiter)
		So(err, ShouldBeNil)
	})

	Convey("prediction", t, func() {
		testData := []struct {
			userId   int
			itemId   int
			expected float64
		}{
			{429, 588, 1.},
			{429, 22, 1.},
			{107, 1, 1.},
			{107, 2, 1.},
			{191, 39, 0.},
			{11, 1391, 0.},
		}

		var (
			yTrue = mat.NewDense(len(testData), 1, nil)
			yPred = mat.NewDense(len(testData), 1, nil)
		)
		rankCtx := context.Background()
		for i, test := range testData {
			score, err := rcmd.Rank(rankCtx, model, test.userId, []int{test.itemId})
			So(err, ShouldBeNil)

			fmt.Printf("userId:%d, itemId:%d, expected:%f, pred:%f\n",
				test.userId, test.itemId, test.expected, score[0].Score)
			//So(pred.At(0, 0), ShouldAlmostEqual, test.expected)
			yTrue.Set(i, 0, test.expected)
			yPred.Set(i, 0, score[0].Score)
		}

		rocAuc := metrics.ROCAUCScore(yTrue, yPred, "", nil)
		fmt.Printf("rocAuc:%f\n", rocAuc)
	})

	Convey("test set ROC AUC", t, func() {
		testCount := 20888
		rows, err := db.Query(
			"SELECT userId, movieId, rating FROM ratings_test ORDER BY timestamp, userId ASC LIMIT ?", testCount)
		So(err, ShouldBeNil)
		var (
			userId       int
			itemId       int
			rating       float64
			yTrue        = mat.NewDense(testCount, 1, nil)
			userAndItems [][2]int
		)
		for i := 0; rows.Next(); i++ {
			err = rows.Scan(&userId, &itemId, &rating)
			if err != nil {
				t.Errorf("scan error: %v", err)
			}
			yTrue.Set(i, 0, BinarizeLabel(rating))
			userAndItems = append(userAndItems, [2]int{userId, itemId})
		}
		batchPredictCtx := context.Background()
		yPred, err := rcmd.BatchPredict(batchPredictCtx, model, userAndItems)
		So(err, ShouldBeNil)
		rocAuc := metrics.ROCAUCScore(yTrue, yPred, "", nil)
		rowCount, _ := yTrue.Dims()
		fmt.Printf("rocAuc on test set %d: %f\n", rowCount, rocAuc)
	})
}
