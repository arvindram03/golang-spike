
-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
create table sample (
  id integer primary key,
  data text
);

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back

drop table sample;