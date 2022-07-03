# edgeRec

All in one Recommendation System running on Edge device (Android/iOS/IoT device etc.)


# Features

- [x] Parameter Server based Online Learning
- [x] Training & Inference all in one binary powered by golang
- Databases support
  - [x] MySQL support
  - [x] SQLite support
  - [ ] Database Aggregation accelerated Feature Normalization
- Feature Engineering
  - [ ] Rule based FE config
  - [ ] DeepL based Auto Feature Engineering
- Demo
  - [ ] Android demo
  - [ ] iOS demo

# Thanks

To make this project work, quite a lot of code are copied and modified from the following libraries:
- Neural Network & Parameter Server: 
  - [go-deep](https://github.com/patrikeh/go-deep)
  - [gorgonia](https://github.com/gorgonia/gorgonia)
- Feature Engineering:
  - [go-featureprocessing](https://github.com/nikolaydubina/go-featureprocessing)
  - [featuremill](https://github.com/dustin-decker/featuremill)
- FastAPI like framework:
  - [go-fastapi](https://github.com/sashabaranov/go-fastapi)

# Papers related

- [EdgeRec: Recommender System on Edge in Mobile Taobao](https://arxiv.org/abs/2005.08416) // not very identical implementation