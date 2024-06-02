# eth-validator-api

Simple HTTP API fronting an ethereum validator, made with Go 1.22.3

## Quick start

```bash
# Run go get in the repository root
go get

# Optionally run tests 
go test

# Generate API docs and start the api
go run main.go

# API server will start, and should be accessible at localhost:8080

# After the server is running, Swagger UI will be available at http://localhost:8080/swagger/index.html
```

## API Endpoints

API serves two endpoints, `blockreward`, available at `http://localhost:8080/blockreward/` when running locally, and `syncduties` available at `http://localhost:8080/syncduties/` when running locally. Both endpoints take a slot number on the Ethereum blockchain as the only parameter.

### Examples

```bash
# blockreward
curl -X 'GET' 'http://localhost:8080/blockreward/8000000' -H 'accept: application/json'


# syncduties
curl -X 'GET' 'http://localhost:8080/syncduties/8000000' -H 'accept: application/json'
```

### Implementation details

API is built with the [Gin](https://github.com/gin-gonic/gin) framework, using [Swag](https://github.com/swaggo/swag) for API docs generation and [gin-swagger](https://github.com/swaggo/gin-swagger) for automatic swagger UI generation.

Where aplicable caching and parallelism are used to speed up execution, so repeated queries should return in a fraction of the time required for the original ones. Cache expiry has been put out of scope of this effort.

#### MEV

Repository contains a `mev.json` files with a list of know mev-builder accounts, these are use to decide if a block is a MEV block. These were scraped from [Etherscan](https://etherscan.io/accounts/label/mev-builder).

`blockreward` endpoint differs from the specification slightly, it includes the `status` field indicating if a block is a MEV block (`true`) or a vanilla block (`false`), it also inludes the `blockReward` and `mevReward` fields. Block reward is the "classic" reward composed of a static reward, transaction fees, and burnt fees. `mevReward` is the reward payed out by the mev-builder account to the validator/relayer. For the most part, there are some deviations from these principles which are also deemed out of scope of this effort.