#!/usr/bin/env python3
"""Adversarial tests for the fail-closed Yoda live-Agent trace evaluator."""

from __future__ import annotations

import unittest
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from eval_live_agent_trace import evaluate  # noqa: E402


def text(value: str) -> dict[str, str]:
    return {"type": "text", "text": value}


def correlated_trace() -> list[dict]:
    return [
        {"type": "system", "subtype": "hook_response", "hook_event": "SessionStart", "exit_code": 0, "outcome": "success"},
        {"type": "system", "subtype": "hook_response", "hook_event": "UserPromptSubmit", "exit_code": 0, "output": "<!-- maestro:agent-route -->\n- `yoda`"},
        {"type": "assistant", "message": {"content": [{"type": "tool_use", "id": "toolu_yoda", "name": "Agent", "input": {"subagent_type": "yoda"}}]}},
        {"type": "system", "subtype": "hook_response", "hook_event": "PreToolUse", "exit_code": 0, "output": "dispatch yoda"},
        {"type": "assistant", "parent_tool_use_id": "toolu_yoda", "message": {"content": [text("CANARIO_YODA_OK")]}},
        {"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": "toolu_yoda", "content": "CANARIO_YODA_OK"}]}},
        {"type": "assistant", "message": {"content": [text("CANARIO_HUB_OK")]}},
    ]


class EvaluateTraceTest(unittest.TestCase):
    def test_prompt_tokens_do_not_fake_agent_calls(self):
        checks, tool_use_ids = evaluate([
            {"type": "user", "message": {"content": [text("yoda CANARIO_YODA_OK")]}},
            {"type": "assistant", "message": {"content": [text("CANARIO_YODA_OK\nCANARIO_HUB_OK")]}},
        ], 0)
        self.assertEqual(tool_use_ids, {"yoda": None})
        self.assertFalse(checks["exactly_one_agent_tool_yoda_observed"])
        self.assertFalse(checks["yoda_return_correlated_to_tool_use"])

    def test_correlated_yoda_trace_passes(self):
        checks, tool_use_ids = evaluate(correlated_trace(), 0)
        self.assertEqual(tool_use_ids, {"yoda": "toolu_yoda"})
        self.assertTrue(all(checks.values()), checks)

    def test_route_hook_error_fails(self):
        events = correlated_trace()
        events[1]["outcome"] = "error"
        events[1]["exit_code"] = 2
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["user_prompt_route_hook_returned_yoda"])

    def test_pretool_hook_error_fails(self):
        events = correlated_trace()
        events[3]["outcome"] = "error"
        events[3]["exit_code"] = 2
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["yoda_pretool_hook_correlated"])

    def test_extra_agent_call_fails_exact_contract(self):
        events = correlated_trace()
        events.insert(-1, {"type": "assistant", "message": {"content": [{"type": "tool_use", "id": "toolu_extra", "name": "Agent", "input": {"subagent_type": "gamma-guardian"}}]}})
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["exactly_one_agent_tool_call_total"])

    def test_mismatched_result_id_fails(self):
        events = correlated_trace()
        events[4]["parent_tool_use_id"] = "toolu_wrong"
        events[5]["message"]["content"][0]["tool_use_id"] = "toolu_wrong"
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["yoda_return_correlated_to_tool_use"])
        self.assertFalse(checks["hub_return_after_yoda_result"])


if __name__ == "__main__":
    unittest.main()
