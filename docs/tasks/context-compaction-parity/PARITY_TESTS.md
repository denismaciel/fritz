# adopted Codex compaction parity tests

- [x] reconstruct_history_matches_live_compactions
- [x] build_token_limited_compacted_history_truncates_overlong_user_messages
- [x] build_token_limited_compacted_history_appends_summary_message
- [x] manual_compact_uses_custom_prompt
- [x] multiple_auto_compact_per_task_runs_after_token_limit_hit
- [x] auto_compact_runs_after_resume_when_token_usage_is_over_limit
- [x] auto_compact_persists_rollout_entries
- [x] manual_compact_retries_after_context_window_error
- [x] manual_compact_twice_preserves_latest_user_messages
- [x] auto_compact_allows_multiple_attempts_when_interleaved_with_other_turn_events
- [x] snapshot_request_shape_mid_turn_continuation_compaction
- [x] snapshot_request_shape_pre_turn_compaction_context_window_exceeded
