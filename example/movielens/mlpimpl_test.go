package movielens

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/auxten/edgeRec/nn/metrics"
	rcmd "github.com/auxten/edgeRec/recommend"
	log "github.com/sirupsen/logrus"
	. "github.com/smartystreets/goconvey/convey"
	"gonum.org/v1/gonum/mat"
)

func TestSimpleMLPOnMovielens(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	rand.Seed(42)

	rcmd.DebugUserId = 429
	//rcmd.DebugItemId = 588

	var (
		movielens = &MovielensRec{
			DataPath: "movielens.db",
			//SampleCnt: 79948,
			SampleCnt: 10000,
		}
		model rcmd.Predictor
		err   error
	)

	Convey("Train din model", t, func() {
		mlpImpl := &mlpImpl{
			predBatchSize: 100,
			batchSize:     20,
			epochs:        200,
		}
		trainCtx := context.Background()
		model, err = rcmd.Train(trainCtx, movielens, mlpImpl)
		So(err, ShouldBeNil)
		So(model, ShouldNotBeNil)
	})

	Convey("Predict din model", t, func() {
		testCount := 200
		rows, err := db.Query(
			"SELECT userId, movieId, rating, timestamp FROM ratings_test ORDER BY timestamp, userId ASC LIMIT ?", testCount)
		So(err, ShouldBeNil)
		var (
			userId     int
			itemId     int
			rating     float64
			timestamp  int64
			yTrue      = mat.NewDense(testCount, 1, nil)
			sampleKeys = make([]rcmd.Sample, 0, testCount)
		)
		for i := 0; rows.Next(); i++ {
			err = rows.Scan(&userId, &itemId, &rating, &timestamp)
			if err != nil {
				t.Errorf("scan error: %v", err)
			}
			yTrue.Set(i, 0, BinarizeLabel(rating))
			sampleKeys = append(sampleKeys, rcmd.Sample{userId, itemId, 0, timestamp})
		}
		batchPredictCtx := context.Background()
		yPred, err := rcmd.BatchPredict(batchPredictCtx, model, sampleKeys)
		So(err, ShouldBeNil)
		rocAuc := metrics.ROCAUCScore(yTrue, yPred, "", nil)
		rowCount, _ := yTrue.Dims()
		fmt.Printf("rocAuc on test set %d: %f\n", rowCount, rocAuc)
	})
}