// Copyright © 2020 wego authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"io"

	"github.com/auxten/edgeRec/feature/embedding/model/modelutil/matrix"
	"github.com/auxten/edgeRec/feature/embedding/model/modelutil/vector"
)

type Model interface {
	Train(<-chan string) error
	Save(io.Writer, vector.Type) error
	WordVector(vector.Type) *matrix.Matrix
	GenEmbeddingMap() (map[string][]float64, error)
	GenEmbeddingMap32() (map[string][]float32, error)
	EmbeddingByWord(word string) ([]float64, bool)
}
