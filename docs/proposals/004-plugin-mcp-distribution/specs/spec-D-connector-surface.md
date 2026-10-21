# Spec D — Proposta futura de connector

**OUT OF v0.3.0.** Default do epic #428; sem promessa de entrega futura.

A proposta original registraria um eventual servidor MCP em outra superfície
Claude por configuração consentida, preservando configurações existentes.
Isso exige a [Spec C](spec-C-mcp-server.md), definição de concorrência sobre
estado e comunicação explícita das capacidades ausentes: hooks, agentes e
orientação de sessão não aparecem automaticamente por haver tools MCP.

O **Code tab do Claude desktop** já é a superfície alvo da v0.3.0. Usá-lo não
requer criar um connector ou editar `claude_desktop_config.json` para Maestro.
Chat, Cowork, web, mobile e servidor remoto/OAuth ficam fora destes gates.
Nenhum comando setup-connector nem alteração de outro app é prometido aqui.
