#!/usr/bin/env python3
"""Correlate a Claude stream trace into a fail-closed Agent canary receipt."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


def _blocks(event: dict[str, Any]) -> list[dict[str, Any]]:
    message = event.get("message") or {}
    content = message.get("content") or []
    return content if isinstance(content, list) else []


def _text_from(value: Any) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, list):
        return "\n".join(_text_from(item) for item in value)
    if isinstance(value, dict):
        if value.get("type") == "text":
            return str(value.get("text") or "")
        return "\n".join(_text_from(item) for item in value.values())
    return ""


def _has_exact_line(value: Any, token: str) -> bool:
    return any(line.strip() == token for line in _text_from(value).splitlines())


def evaluate(
    events: list[dict[str, Any]], claude_rc: int
) -> tuple[dict[str, bool], dict[str, str | None]]:
    """Return fail-closed checks and the observed Darwin/Yoda tool-use ids."""
    route_hook = False
    session_start = False
    agent_calls: dict[str, list[tuple[int, str | None]]] = {
        "darwin": [],
        "yoda": [],
    }
    all_agent_calls: list[tuple[int, str | None, str | None]] = []

    for index, event in enumerate(events):
        if event.get("type") == "system" and event.get("subtype") == "hook_response":
            hook_event = event.get("hook_event")
            hook_text = "\n".join(str(event.get(key) or "") for key in ("output", "stdout"))
            if (
                hook_event == "SessionStart"
                and event.get("outcome") != "error"
                and event.get("exit_code", 0) == 0
            ):
                session_start = True
            if (
                hook_event == "UserPromptSubmit"
                and event.get("outcome") != "error"
                and event.get("exit_code", 0) == 0
                and "<!-- maestro:agent-route -->" in hook_text
                and "`darwin`" in hook_text
                and "`yoda`" in hook_text
            ):
                route_hook = True
        if event.get("type") == "assistant":
            for block in _blocks(event):
                agent_input = block.get("input") or {}
                if block.get("type") == "tool_use" and block.get("name") == "Agent":
                    all_agent_calls.append(
                        (index, block.get("id"), agent_input.get("subagent_type"))
                    )
                if (
                    block.get("type") == "tool_use"
                    and block.get("name") == "Agent"
                    and agent_input.get("subagent_type") in agent_calls
                ):
                    agent_calls[agent_input["subagent_type"]].append(
                        (index, block.get("id"))
                    )

    call_indexes: dict[str, int] = {}
    call_ids: dict[str, str | None] = {}
    result_indexes: dict[str, int] = {}
    agent_returns: dict[str, bool] = {}
    pretool_hooks: dict[str, bool] = {}
    for agent, token in (
        ("darwin", "CANARIO_DARWIN_OK"),
        ("yoda", "CANARIO_YODA_OK"),
    ):
        call_index, call_id = (
            agent_calls[agent][0]
            if len(agent_calls[agent]) == 1
            else (-1, None)
        )
        call_indexes[agent] = call_index
        call_ids[agent] = call_id
        result_index = -1
        agent_return = False
        if call_id:
            for index, event in enumerate(events):
                if (
                    event.get("type") == "assistant"
                    and event.get("parent_tool_use_id") == call_id
                    and _has_exact_line(_blocks(event), token)
                ):
                    agent_return = True
                    result_index = max(result_index, index)
                if event.get("type") == "user":
                    for block in _blocks(event):
                        if (
                            block.get("type") == "tool_result"
                            and block.get("tool_use_id") == call_id
                        ):
                            result_index = max(result_index, index)
                            if _has_exact_line(block.get("content"), token):
                                agent_return = True
        result_indexes[agent] = result_index
        agent_returns[agent] = agent_return

        pretool_hook = False
        if call_index >= 0:
            upper = result_index if result_index >= 0 else len(events) - 1
            for event in events[call_index + 1 : upper + 1]:
                if (
                    event.get("type") == "system"
                    and event.get("subtype") == "hook_response"
                    and event.get("hook_event") == "PreToolUse"
                    and event.get("outcome") != "error"
                    and event.get("exit_code", 0) == 0
                ):
                    hook_text = "\n".join(
                        str(event.get(key) or "") for key in ("output", "stdout")
                    )
                    if agent in hook_text:
                        pretool_hook = True
        pretool_hooks[agent] = pretool_hook

    darwin_before_yoda = (
        result_indexes["darwin"] >= 0
        and call_indexes["yoda"] > result_indexes["darwin"]
    )
    hub_return = False
    if result_indexes["yoda"] >= 0:
        for event in events[result_indexes["yoda"] + 1 :]:
            if (
                event.get("type") == "assistant"
                and not event.get("parent_tool_use_id")
                and _has_exact_line(_blocks(event), "CANARIO_HUB_OK")
            ):
                hub_return = True

    checks = {
        "claude_exit_zero": claude_rc == 0,
        "session_start_hook_succeeded": session_start,
        "user_prompt_route_hook_returned_darwin_and_yoda": route_hook,
        "exactly_two_agent_tool_calls_total": len(all_agent_calls) == 2,
        "exactly_one_agent_tool_darwin_observed": len(agent_calls["darwin"]) == 1,
        "darwin_pretool_hook_correlated": pretool_hooks["darwin"],
        "darwin_return_correlated_to_tool_use": agent_returns["darwin"],
        "yoda_called_after_darwin_result": darwin_before_yoda,
        "exactly_one_agent_tool_yoda_observed": len(agent_calls["yoda"]) == 1,
        "yoda_pretool_hook_correlated": pretool_hooks["yoda"],
        "yoda_return_correlated_to_tool_use": agent_returns["yoda"],
        "hub_return_after_yoda_result": hub_return,
    }
    return checks, call_ids


def _load_events(path: Path) -> list[dict[str, Any]]:
    events = []
    for line in path.read_text(encoding="utf-8").splitlines():
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            events.append(value)
    return events


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--trace", type=Path, required=True)
    parser.add_argument("--receipt", type=Path, required=True)
    parser.add_argument("--platform", required=True)
    parser.add_argument("--architecture", required=True)
    parser.add_argument("--release-sha256", required=True)
    parser.add_argument("--claude-version", required=True)
    parser.add_argument("--claude-rc", type=int, required=True)
    args = parser.parse_args()

    checks, agent_call_ids = evaluate(_load_events(args.trace), args.claude_rc)
    verdict = "PASS" if all(checks.values()) else "FAIL"
    receipt = {
        "schema_version": 1,
        "evidence_kind": "maestro_claude_agent_live",
        "platform": args.platform,
        "architecture": args.architecture,
        "release_sha256": args.release_sha256,
        "claude_code": args.claude_version,
        "agent_tool_use_ids": agent_call_ids,
        "checks": checks,
        "verdict": verdict,
        "limits": [
            "synthetic prompt only",
            "not release publication or signing evidence",
        ],
    }
    with args.receipt.open("x", encoding="utf-8", newline="\n") as handle:
        json.dump(receipt, handle, ensure_ascii=False, indent=2)
        handle.write("\n")
    print(verdict)
    return 0 if verdict == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
