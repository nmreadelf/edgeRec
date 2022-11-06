package din

import (
	"fmt"
	"math"

	"github.com/auxten/edgeRec/nn/metrics"
	rcmd "github.com/auxten/edgeRec/recommend"
	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/mat"
	"gopkg.in/cheggaaa/pb.v1"
	G "gorgonia.org/gorgonia"
	"gorgonia.org/tensor"
)

var DT = tensor.Float32

const (
	// magic numbers for din paper
	att0_1 = 36
	mlp0_1 = 200
	mlp1_2 = 80
)

type Model interface {
	learnable() G.Nodes
	Fwd(xUserProfile, ubMatrix, xItemFeature, xCtxFeature *G.Node, batchSize, uBehaviorSize, uBehaviorDim int) (err error)
	Out() *G.Node
	In() G.Nodes
	Graph() *G.ExprGraph
	Marshal() (data []byte, err error)
	Vm() G.VM
	SetVM(vm G.VM)
}

func Train(uProfileDim, uBehaviorSize, uBehaviorDim, iFeatureDim, cFeatureDim int,
	numExamples, batchSize, epochs, earlyStop int,
	si *rcmd.SampleInfo,
	inputs, targets tensor.Tensor,
	//testInputs, testTargets tensor.Tensor,
	m Model,
) (err error) {
	g := m.Graph()
	xUserProfile := G.NewMatrix(g, DT, G.WithShape(batchSize, uProfileDim), G.WithName("xUserProfile"))
	//xUserBehaviors := G.NewTensor(g, DT, 3, G.WithShape(batchSize, uBehaviorSize, uBehaviorDim), G.WithName("xUserBehaviors"))
	xUserBehaviorMatrix := G.NewMatrix(g, DT, G.WithShape(batchSize, uBehaviorSize*uBehaviorDim), G.WithName("xUserBehaviorMatrix"))
	xItemFeature := G.NewMatrix(g, DT, G.WithShape(batchSize, iFeatureDim), G.WithName("xItemFeature"))
	xCtxFeature := G.NewMatrix(g, DT, G.WithShape(batchSize, cFeatureDim), G.WithName("xCtxFeature"))
	y := G.NewTensor(g, DT, 2, G.WithShape(batchSize, 1), G.WithName("y"))
	//m := NewDinNet(g, uProfileDim, uBehaviorSize, uBehaviorDim, iFeatureDim, cFeatureDim)
	if err = m.Fwd(xUserProfile, xUserBehaviorMatrix, xItemFeature, xCtxFeature, batchSize, uBehaviorSize, uBehaviorDim); err != nil {
		log.Fatalf("%+v", err)
	}

	//losses := G.Must(G.HadamardProd(G.Must(G.Neg(G.Must(G.Log(m.out)))), y))
	//losses := G.Must(G.Square(G.Must(G.Sub(m.Out(), y))))
	positive := G.Must(G.HadamardProd(G.Must(G.Log(m.Out())), y))
	negative := G.Must(G.HadamardProd(G.Must(G.Log(G.Must(G.Sub(G.NewConstant(float32(1.0+1e-8)), m.Out())))), G.Must(G.Sub(G.NewConstant(float32(1.0)), y))))
	//negative := G.Must(G.Log(G.Must(G.Sub(G.NewConstant(float32(1.000000001)), m.Out()))))
	cost := G.Must(G.Neg(G.Must(G.Mean(G.Must(G.Add(positive, negative))))))

	// we want to track costs
	//var costVal G.Value
	//G.Read(cost, &costVal)
	//
	//var yOut G.Value
	//G.Read(m.Out(), &yOut)

	if _, err = G.Grad(cost, m.learnable()...); err != nil {
		log.Fatal(err)
	}

	// debug
	//ioutil.WriteFile("fullGraph.dot", []byte(g.ToDot()), 0644)
	// log.Printf("%v", prog)
	// logger := log.New(os.Stderr, "", 0)
	// vm := gorgonia.NewTapeMachine(g, gorgonia.BindDualValues(m.learnable()...), gorgonia.WithLogger(logger), gorgonia.WithWatchlist())

	prog, locMap, err := G.Compile(g)
	if err != nil {
		log.Fatal(err)
	}
	//log.Printf("%v", prog)

	vm := G.NewTapeMachine(g,
		G.WithPrecompiled(prog, locMap),
		G.BindDualValues(m.learnable()...),
		//G.TraceExec(),
		//G.WithInfWatch(),
		//G.WithNaNWatch(),
		//G.WithLogger(log.New(os.Stderr, "", 0)),
		//G.WithWatchlist(m.mlp2),
	)
	m.SetVM(vm)

	//solver := G.NewRMSPropSolver(G.WithBatchSize(float32(batchSize)))
	//solver := G.NewVanillaSolver(G.WithBatchSize(float32(batchSize)), G.WithLearnRate(0.001))
	//solver := G.NewBarzilaiBorweinSolver(G.WithBatchSize(float32(batchSize)), G.WithLearnRate(0.001))
	//solver := G.NewAdaGradSolver(G.WithBatchSize(float32(batchSize)), G.WithLearnRate(0.001))
	//solver := G.NewMomentum(G.WithBatchSize(float32(batchSize)), G.WithLearnRate(0.001))
	solver := G.NewAdamSolver(G.WithLearnRate(0.01), G.WithBatchSize(float64(batchSize)), G.WithL2Reg(0.0001))
	//defer func() {
	//	vm.Close()
	//	m.SetVM(nil)
	//}()
	// pprof
	// handlePprof(sigChan, doneChan)

	batches := numExamples / batchSize
	log.Printf("Batches %d", batches)
	bar := pb.New(batches)
	var (
		bestCost  float32 = math.MaxFloat32
		noImprove int
	)

	for i := 0; i < epochs; i++ {
		bar.Prefix(fmt.Sprintf("Epoch %d", i))
		bar.Set(0)
		bar.Start()
		for b := 0; b < batches; b++ {
			start := b * batchSize
			end := start + batchSize
			if start >= numExamples {
				break
			}
			if end > numExamples {
				end = numExamples
			}

			var (
				xUserProfileVal   tensor.Tensor
				xUserBehaviorsVal tensor.Tensor
				xItemFeatureVal   tensor.Tensor
				xCtxFeatureVal    tensor.Tensor
				yVal              tensor.Tensor
			)

			if xUserProfileVal, err = inputs.Slice([]tensor.Slice{G.S(start, end), G.S(si.UserProfileRange[0], si.UserProfileRange[1])}...); err != nil {
				log.Fatalf("Unable to slice xUserProfileVal %v", err)
			}
			if err = G.Let(xUserProfile, xUserProfileVal); err != nil {
				log.Fatalf("Unable to let xUserProfileVal %v", err)
			}

			if xUserBehaviorsVal, err = inputs.Slice([]tensor.Slice{G.S(start, end), G.S(si.UserBehaviorRange[0], si.UserBehaviorRange[1])}...); err != nil {
				log.Fatalf("Unable to slice xUserBehaviorsVal %v", err)
			}
			if err = G.Let(xUserBehaviorMatrix, xUserBehaviorsVal); err != nil {
				log.Fatalf("Unable to let xUserBehaviorsVal %v", err)
			}

			if xItemFeatureVal, err = inputs.Slice([]tensor.Slice{G.S(start, end), G.S(si.ItemFeatureRange[0], si.ItemFeatureRange[1])}...); err != nil {
				log.Fatalf("Unable to slice xItemFeatureVal %v", err)
			}
			if err = G.Let(xItemFeature, xItemFeatureVal); err != nil {
				log.Fatalf("Unable to let xItemFeatureVal %v", err)
			}

			if xCtxFeatureVal, err = inputs.Slice([]tensor.Slice{G.S(start, end), G.S(si.CtxFeatureRange[0], si.CtxFeatureRange[1])}...); err != nil {
				log.Fatalf("Unable to slice xCtxFeatureVal %v", err)
			}
			if err = G.Let(xCtxFeature, xCtxFeatureVal); err != nil {
				log.Fatalf("Unable to let xCtxFeatureVal %v", err)
			}

			if yVal, err = targets.Slice(G.S(start, end)); err != nil {
				log.Fatalf("Unable to slice y %v", err)
			}
			if err = G.Let(y, yVal); err != nil {
				log.Fatalf("Unable to let y %v", err)
			}

			if err = vm.RunAll(); err != nil {
				log.Fatalf("Failed at epoch  %d, batch %d. Error: %v", i, b, err)
			}
			if err = solver.Step(G.NodesToValueGrads(m.learnable())); err != nil {
				log.Fatalf("Failed to update nodes with gradients at epoch %d, batch %d. Error %v", i, b, err)
			}
			vm.Reset()
			bar.Increment()
		}
		costVal := cost.Value().Data().(float32)
		if costVal < bestCost {
			bestCost = costVal
			noImprove = 0
		} else {
			noImprove++
		}
		log.Printf("Epoch %d | noImprove %d | cost %v", i, noImprove, costVal)
		if earlyStop != 0 && noImprove >= earlyStop {
			log.Printf("Early stop at epoch %d", i)
			break
		}
		//log.Printf("Test accuracy %v | rocauc %v")
	}
	return
}

func InitForwardOnlyVm(uProfileDim, uBehaviorSize, uBehaviorDim, iFeatureDim, cFeatureDim int,
	batchSize int,
	m Model,
) (err error) {
	g := m.Graph()
	xUserProfile := G.NewMatrix(g, DT, G.WithShape(batchSize, uProfileDim), G.WithName("xUserProfile"))
	xUserBehaviorMatrix := G.NewMatrix(g, DT, G.WithShape(batchSize, uBehaviorSize*uBehaviorDim), G.WithName("xUserBehaviorMatrix"))
	xItemFeature := G.NewMatrix(g, DT, G.WithShape(batchSize, iFeatureDim), G.WithName("xItemFeature"))
	xCtxFeature := G.NewMatrix(g, DT, G.WithShape(batchSize, cFeatureDim), G.WithName("xCtxFeature"))
	if err = m.Fwd(xUserProfile, xUserBehaviorMatrix, xItemFeature, xCtxFeature,
		batchSize, uBehaviorSize, uBehaviorDim); err != nil {
		return
	}
	prog, locMap, err := G.Compile(g)
	if err != nil {
		return
	}
	//log.Printf("%v", prog)

	vm := G.NewTapeMachine(g,
		G.WithPrecompiled(prog, locMap),
	)
	m.SetVM(vm)

	return
}

func Predict(m Model, numExamples, batchSize int, si *rcmd.SampleInfo, inputs tensor.Tensor) (y []float32, err error) {
	//input nodes
	inputNodes := m.In()
	xUserProfile := inputNodes[0]
	xUbMatrix := inputNodes[1]
	xItemFeature := inputNodes[2]
	xCtxFeature := inputNodes[3]

	//output node
	outputNode := m.Out()

	//vm
	vm := m.Vm()

	batches := numExamples / batchSize

	for b := 0; b <= batches; b++ {
		start := b * batchSize
		end := start + batchSize
		if start >= numExamples {
			break
		}
		if end > numExamples {
			end = numExamples
		}

		var (
			xUserProfileVal   tensor.Tensor
			xUserBehaviorsVal tensor.Tensor
			xItemFeatureVal   tensor.Tensor
			xCtxFeatureVal    tensor.Tensor
		)

		if xUserProfileVal, err = inputs.Slice([]tensor.Slice{G.S(start, start+batchSize), G.S(si.UserProfileRange[0], si.UserProfileRange[1])}...); err != nil {
			log.Errorf("Unable to slice xUserProfileVal %v", err)
			return nil, err
		}
		if err = G.Let(xUserProfile, xUserProfileVal); err != nil {
			log.Errorf("Unable to let xUserProfileVal %v", err)
			return nil, err
		}

		if xUserBehaviorsVal, err = inputs.Slice([]tensor.Slice{G.S(start, start+batchSize), G.S(si.UserBehaviorRange[0], si.UserBehaviorRange[1])}...); err != nil {
			log.Errorf("Unable to slice xUserBehaviorsVal %v", err)
			return nil, err
		}
		if err = G.Let(xUbMatrix, xUserBehaviorsVal); err != nil {
			log.Errorf("Unable to let xUserBehaviorsVal %v", err)
			return nil, err
		}

		if xItemFeatureVal, err = inputs.Slice([]tensor.Slice{G.S(start, start+batchSize), G.S(si.ItemFeatureRange[0], si.ItemFeatureRange[1])}...); err != nil {
			log.Errorf("Unable to slice xItemFeatureVal %v", err)
			return nil, err
		}
		if err = G.Let(xItemFeature, xItemFeatureVal); err != nil {
			log.Errorf("Unable to let xItemFeatureVal %v", err)
			return nil, err
		}

		if xCtxFeatureVal, err = inputs.Slice([]tensor.Slice{G.S(start, start+batchSize), G.S(si.CtxFeatureRange[0], si.CtxFeatureRange[1])}...); err != nil {
			log.Errorf("Unable to slice xCtxFeatureVal %v", err)
			return nil, err
		}
		if err = G.Let(xCtxFeature, xCtxFeatureVal); err != nil {
			log.Errorf("Unable to let xCtxFeatureVal %v", err)
			return nil, err
		}

		if err = vm.RunAll(); err != nil {
			log.Errorf("Failed at batch %d. Error: %v", b, err)
			return nil, err
		}

		//get y
		yVal := outputNode.Value().Data().([]float32)
		for i := 0; i < end-start; i++ {
			y = append(y, yVal[i])
		}
		//y = append(y, yVal...)
		vm.Reset()
	}
	return
}

func accuracy(prediction, y []float64) float64 {
	var ok float64
	for i := 0; i < len(prediction); i++ {
		if math.Round(float64(prediction[i]-y[i])) == 0 {
			ok += 1.0
		}
	}
	return ok / float64(len(y))
}

func RocAuc(pred, y []float64) float64 {
	boolY := make([]float64, len(y))
	for i := 0; i < len(y); i++ {
		if y[i] > 0.5 {
			boolY[i] = 1.0
		} else {
			boolY[i] = 0.0
		}
	}
	yTrue := mat.NewDense(len(y), 1, boolY)
	yScore := mat.NewDense(len(pred), 1, pred)

	return metrics.ROCAUCScore(yTrue, yScore, "", nil)
}

func RocAuc32(pred, y []float32) float32 {
	boolY := make([]float64, len(y))
	for i := 0; i < len(y); i++ {
		if y[i] > 0.5 {
			boolY[i] = 1.0
		} else {
			boolY[i] = 0.0
		}
	}
	pred64 := make([]float64, len(pred))
	for i := 0; i < len(pred); i++ {
		pred64[i] = float64(pred[i])
	}
	yTrue := mat.NewDense(len(y), 1, boolY)
	yScore := mat.NewDense(len(pred), 1, pred64)

	return float32(metrics.ROCAUCScore(yTrue, yScore, "", nil))
}
