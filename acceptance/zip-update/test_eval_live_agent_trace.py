import sys
import unittest
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
            {"type": "system", "subtype": "hook_started", "hook_event": "SessionStart"},
        ]
        checks, tool_use_id = evaluate(events, 0)
        self.assertIsNone(tool_use_id)
        self.assertFalse(all(checks.values()))
        self.assertFalse(checks["yoda_return_correlated_to_tool_use"])
        self.assertFalse(checks["hub_return_after_agent_result"])

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
                "output": "<!-- maestro:agent-route -->\n- `yoda`",
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
        checks, tool_use_id = evaluate(events, 0)
        self.assertEqual(tool_use_id, "toolu_yoda")
        self.assertTrue(all(checks.values()), checks)

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
        self.assertFalse(checks["hub_return_after_agent_result"])


if __name__ == "__main__":
    unittest.main()
