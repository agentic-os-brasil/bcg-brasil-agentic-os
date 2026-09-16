import sys
import unittest
from copy import deepcopy
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from eval_live_agent_trace import evaluate  # noqa: E402


def text(value):
    return {"type": "text", "text": value}


class LiveAgentTraceTest(unittest.TestCase):
    def test_prompt_tokens_alone_cannot_pass(self):
        events = [
            {
                "type": "user",
                "message": {"content": [text("yoda CANARIO_YODA_OK CANARIO_HUB_OK")]},
            },
            {
                "type": "assistant",
                "message": {
                    "content": [
                        text("CANARIO_DARWIN_OK\nCANARIO_YODA_OK\nCANARIO_HUB_OK")
                    ]
                },
            },
            {"type": "system", "subtype": "hook_started", "hook_event": "SessionStart"},
        ]
        checks, tool_use_ids = evaluate(events, 0)
        self.assertEqual(tool_use_ids, {"darwin": None, "yoda": None})
        self.assertFalse(all(checks.values()))
        self.assertFalse(checks["darwin_return_correlated_to_tool_use"])
        self.assertFalse(checks["yoda_return_correlated_to_tool_use"])
        self.assertFalse(checks["hub_return_after_yoda_result"])

    def test_correlated_trace_passes(self):
        events = [
            {
                "type": "system",
                "subtype": "hook_response",
                "hook_event": "SessionStart",
                "exit_code": 0,
                "outcome": "success",
            },
            {
                "type": "system",
                "subtype": "hook_response",
                "hook_event": "UserPromptSubmit",
                "exit_code": 0,
                "output": "<!-- maestro:agent-route -->\n- `darwin`\n- `yoda`",
            },
            {
                "type": "assistant",
                "message": {
                    "content": [
                        {
                            "type": "tool_use",
                            "id": "toolu_darwin",
                            "name": "Agent",
                            "input": {"subagent_type": "darwin"},
                        }
                    ]
                },
            },
            {
                "type": "system",
                "subtype": "hook_response",
                "hook_event": "PreToolUse",
                "exit_code": 0,
                "output": "dispatch darwin",
            },
            {
                "type": "assistant",
                "parent_tool_use_id": "toolu_darwin",
                "message": {"content": [text("CANARIO_DARWIN_OK")]},
            },
            {
                "type": "user",
                "message": {
                    "content": [
                        {
                            "type": "tool_result",
                            "tool_use_id": "toolu_darwin",
                            "content": "CANARIO_DARWIN_OK",
                        }
                    ]
                },
            },
            {
                "type": "assistant",
                "message": {
                    "content": [
                        {
                            "type": "tool_use",
                            "id": "toolu_yoda",
                            "name": "Agent",
                            "input": {"subagent_type": "yoda"},
                        }
                    ]
                },
            },
            {
                "type": "system",
                "subtype": "hook_response",
                "hook_event": "PreToolUse",
                "exit_code": 0,
                "output": "dispatch yoda",
            },
            {
                "type": "assistant",
                "parent_tool_use_id": "toolu_yoda",
                "message": {"content": [text("CANARIO_YODA_OK")]},
            },
            {
                "type": "user",
                "message": {
                    "content": [
                        {
                            "type": "tool_result",
                            "tool_use_id": "toolu_yoda",
                            "content": "CANARIO_YODA_OK",
                        }
                    ]
                },
            },
            {"type": "assistant", "message": {"content": [text("CANARIO_HUB_OK")]}},
        ]
        checks, tool_use_ids = evaluate(events, 0)
        self.assertEqual(
            tool_use_ids, {"darwin": "toolu_darwin", "yoda": "toolu_yoda"}
        )
        self.assertTrue(all(checks.values()), checks)

        route_error = deepcopy(events)
        route_error[1]["outcome"] = "error"
        route_error[1]["exit_code"] = 2
        route_checks, _ = evaluate(route_error, 0)
        self.assertFalse(
            route_checks["user_prompt_route_hook_returned_darwin_and_yoda"]
        )

        pretool_error = deepcopy(events)
        pretool_error[3]["outcome"] = "error"
        pretool_error[3]["exit_code"] = 2
        pretool_checks, _ = evaluate(pretool_error, 0)
        self.assertFalse(pretool_checks["darwin_pretool_hook_correlated"])

        extra_agent = deepcopy(events)
        extra_agent.insert(
            -1,
            {
                "type": "assistant",
                "message": {
                    "content": [
                        {
                            "type": "tool_use",
                            "id": "toolu_extra",
                            "name": "Agent",
                            "input": {"subagent_type": "gamma-guardian"},
                        }
                    ]
                },
            },
        )
        extra_checks, _ = evaluate(extra_agent, 0)
        self.assertFalse(extra_checks["exactly_two_agent_tool_calls_total"])

    def test_mismatched_result_id_fails(self):
        events = [
            {
                "type": "assistant",
                "message": {
                    "content": [
                        {
                            "type": "tool_use",
                            "id": "toolu_yoda",
                            "name": "Agent",
                            "input": {"subagent_type": "yoda"},
                        }
                    ]
                },
            },
            {
                "type": "user",
                "message": {
                    "content": [
                        {
                            "type": "tool_result",
                            "tool_use_id": "toolu_other",
                            "content": "CANARIO_YODA_OK",
                        }
                    ]
                },
            },
            {"type": "assistant", "message": {"content": [text("CANARIO_HUB_OK")]}},
        ]
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["yoda_return_correlated_to_tool_use"])
        self.assertFalse(checks["hub_return_after_yoda_result"])

    def test_parallel_yoda_call_fails_sequential_contract(self):
        events = [
            {
                "type": "assistant",
                "message": {
                    "content": [
                        {
                            "type": "tool_use",
                            "id": "toolu_darwin",
                            "name": "Agent",
                            "input": {"subagent_type": "darwin"},
                        },
                        {
                            "type": "tool_use",
                            "id": "toolu_yoda",
                            "name": "Agent",
                            "input": {"subagent_type": "yoda"},
                        },
                    ]
                },
            },
            {
                "type": "user",
                "message": {
                    "content": [
                        {
                            "type": "tool_result",
                            "tool_use_id": "toolu_darwin",
                            "content": "CANARIO_DARWIN_OK",
                        },
                        {
                            "type": "tool_result",
                            "tool_use_id": "toolu_yoda",
                            "content": "CANARIO_YODA_OK",
                        },
                    ]
                },
            },
        ]
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["yoda_called_after_darwin_result"])


if __name__ == "__main__":
    unittest.main()
