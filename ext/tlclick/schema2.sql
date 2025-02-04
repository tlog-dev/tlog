
CREATE TABLE IF NOT EXISTS ingest (
	tlog String COMMENT 'raw event',

	_labels Array(String) COMMENT 'tlog labels',

	ts DateTime64(9, 'UTC') DEFAULT addNanoseconds(toDateTime64(0, 9, 'UTC'), JSONExtractInt(json, '_t')),

	json   String COMMENT 'without labels',
	labels String COMMENT 'json labels',
)
ENGINE Null
;

CREATE TABLE IF NOT EXISTS events (
	tlog String COMMENT 'raw event',

	_labels      Array(String) COMMENT 'tlog labels',
	_labels_hash UInt64        MATERIALIZED cityHash64(_labels),

	ts DateTime64(9, 'UTC') DEFAULT addNanoseconds(toDateTime64(0, 9, 'UTC'), JSONExtractInt(json, '_t')),

	json   String COMMENT 'without labels',
	labels JSON(),

	_s UUID DEFAULT toUUIDOrZero(JSONExtractString(json, '_s')) COMMENT 'span',
	_p UUID DEFAULT toUUIDOrZero(JSONExtractString(json, '_p')) COMMENT 'parent',

	_k FixedString(1) DEFAULT substring(JSONExtractString(json, '_k')), 1, 2) COMMENT 'kind',
	_c String         DEFAULT JSONExtractString(json, '_c')) COMMENT 'caller',
	_e Int64          DEFAULT JSONExtractInt(json, '_e')) COMMENT 'elapsed',
	_l Int8           DEFAULT JSONExtractInt(json, '_l')) COMMENT 'log level',

	_m String DEFAULT JSONExtractInt(json, '_m')) COMMENT 'message',

	kvs JSON(SKIP _s, SKIP _p, SKIP _k, SKIP _c, SKIP _e, SKIP _l, SKIP _m) COMMENT 'json without _* keys',

	minute  DateTime ALIAS        toStartOfMinute(ts),
	hour    DateTime ALIAS        toStartOfHour(ts),
	day     Date     ALIAS        toStartOfDay(ts),
	week    Date     MATERIALIZED toStartOfWeek(ts),
	month   Date     ALIAS        toStartOfMonth(ts),

	db_insert_time DateTime DEFAULT now(),
)
ENGINE MergeTree
ORDER BY (_labels_hash, ts)
PARTITION BY week
;

CREATE MATERIALIZED VIEW IF NOT EXISTS events_mv
TO event
AS SELECT
	tlog,
	_labels,
	ts,
	json,
	labels,
	json AS kvs,
	0
FROM ingest
;

CREATE TABLE IF NOT EXISTS spans (
	_s UUID,
	ts DateTime64(9, 'UTC'),

	_labels_hash UInt64,

	minute  DateTime ALIAS        toStartOfMinute(ts),
	hour    DateTime ALIAS        toStartOfHour(ts),
	day     Date     ALIAS        toStartOfDay(ts),
	week    Date     MATERIALIZED toStartOfWeek(ts),
	month   Date     ALIAS        toStartOfMonth(ts),

	db_insert_time DateTime DEFAULT now(),
)
ENGINE ReplacingMergeTree()
ORDER BY (_s, _labels_hash, ts)
PARTITION BY week
;

CREATE MATERIALIZED VIEW IF NOT EXISTS spans_mv
TO spans
AS SELECT
	_s,
	ts,
	_labels_hash,
	0
FROM events
-- WHERE notEmpty(_s)
;

CREATE TABLE IF NOT EXISTS parent (
	_s UUID,
	_p UUID,
	ts DateTime64(9, 'UTC'),

	_labels_hash UInt64,

	minute  DateTime ALIAS        toStartOfMinute(ts),
	hour    DateTime ALIAS        toStartOfHour(ts),
	day     Date     ALIAS        toStartOfDay(ts),
	week    Date     MATERIALIZED toStartOfWeek(ts),
	month   Date     ALIAS        toStartOfMonth(ts),

	db_insert_time DateTime DEFAULT now(),
)
ENGINE ReplacingMergeTree()
ORDER BY (_s, _p, _labels_hash, ts)
PARTITION BY week
;

CREATE MATERIALIZED VIEW IF NOT EXISTS parent_mv
TO parent
AS SELECT
	_s,
	_p,
	ts,
	_labels_hash,
	0
FROM events
WHERE notEmpty(_p) AND notEmpty(_s)
;

CREATE TABLE IF NOT EXISTS labels (
	labels       JSON(),
	_labels_hash UInt64,

	ts DateTime64(9, 'UTC'),

	minute  DateTime ALIAS        toStartOfMinute(ts),
	hour    DateTime ALIAS        toStartOfHour(ts),
	day     Date     ALIAS        toStartOfDay(ts),
	week    Date     MATERIALIZED toStartOfWeek(ts),
	month   Date     ALIAS        toStartOfMonth(ts),

	db_insert_time DateTime DEFAULT now(),
)
ENGINE ReplacingMergeTree()
ORDER BY (_labels_hash)
PARTITION BY week
;

CREATE MATERIALIZED VIEW IF NOT EXISTS labels_mv
TO labels
AS SELECT
	labels,
	_labels_hash,
	ts,
	0
FROM events
;
