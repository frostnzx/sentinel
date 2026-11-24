.PHONY: generate generate-go generate-ts

generate: generate-go generate-ts

generate-go:
	cd backend && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1 --config oapi-codegen.yaml ../contracts/openapi.json

generate-ts:
	cd frontend && npm run generate:api
