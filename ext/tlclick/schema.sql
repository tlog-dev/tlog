
CREATE TABLE IF NOT EXISTS events (
	tlog String,
	json String,

	ts DateTime64(9, 'UTC'),

	labels       Array(String),
	_labels      Array(String),
	_labels_hash UInt64 MATERIALIZED cityHash64(_labels),

	_s UUID, -- MATERIALIZED toUUIDOrZero(visitParamExtractString(json, '_s')),
	_p UUID, -- MATERIALIZED toUUIDOrZero(visitParamExtractString(json, '_p')),

	_refs Map(String, UUID),

	_k FixedString(1) MATERIALIZED visitParamExtractString(json, '_k'),
	_m String         MATERIALIZED visitParamExtractString(json, '_m'),
	_l Int8           MATERIALIZED visitParamExtractInt(json, '_l'),
	_e Int64          MATERIALIZED visitParamExtractInt(json, '_e'),

	err String MATERIALIZED visitParamExtractString(json, 'err'),

	kvs Map(String, String) MATERIALIZED mapFromArrays(
		(arrayFilter(
			k -> k NOT IN ('_s', '_p', '_t', '_c', '_k', '_m', '_l', '_e', 'err'),
			JSONExtractKeys(json)
		) AS kv_keys),
		arrayMap(k -> JSONExtractRaw(json, k), kv_keys)
	),

	minute  DateTime ALIAS        toStartOfMinute(ts),
	hour    DateTime ALIAS        toStartOfHour(ts),
	day     Date     MATERIALIZED toStartOfDay(ts),
	week    Date     ALIAS        toStartOfWeek(ts),
	month   Date     ALIAS        toStartOfMonth(ts),
	year    Date     ALIAS        toStartOfYear(ts),
)
ENGINE MergeTree
ORDER BY ts
PARTITION BY day
;

CREATE TABLE IF NOT EXISTS spans (
	span   UUID,

	parent SimpleAggregateFunction(max, UUID),
	refs   AggregateFunction(groupUniqArray, Array(UUID)),

	start DateTime64(9, 'UTC'),
	end   DateTime64(9, 'UTC'),

	elapsed    Int64,
	finish_err String,

	day DateTime,
)
ENGINE AggregatingMergeTree
ORDER BY span
PARTITION BY day
;

CREATE TABLE IF NOT EXISTS labels (
	label String,
	span  UUID,

	day DateTime,
)
ENGINE AggregatingMergeTree
ORDER BY (label, span)
PARTITION BY day
;
