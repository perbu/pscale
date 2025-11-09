-- +goose Up
-- +goose StatementBegin
CREATE TABLE test_data (
    id BIGSERIAL PRIMARY KEY,
    data TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE test_data;
-- +goose StatementEnd
