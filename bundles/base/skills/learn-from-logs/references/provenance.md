# Proveniência

O log de conversa contém três tipos de evidência que é fácil confundir. Confundi-los é o **modo de falha nº 1** desta skill — produz um perfil que descreve os hábitos do assistente de volta para o dono, em vez do contrário.

## Tags

- `[OP]` — o dono afirmou ou decidiu explicitamente. Evidência forte.
- `[ACEITO]` — o assistente sugeriu e o dono não contestou. Evidência fraca. Silêncio ≠ endosso: o dono pode estar com pressa, pode não ter lido com atenção, pode ter aceitado por conveniência local naquele momento.
- `[TERCEIRO]` — veio de sócio, cliente, revisor, comentário colado no chat. Registrar quem disse, quando der para saber.

## Regras

1. **Toda evidência recebe tag.** Sem tag, não entra em rascunho nem em candidato.
2. **`[ACEITO]` sozinho nunca vira escrita confirmada.** Fica como hipótese, com contador. Duas fontes `[ACEITO]` continuam sendo dois sinais fracos, não uma regra — só `[OP]`/`[TERCEIRO]` recorrente (≥2 sessões distintas) cruza a barra.
3. **Descartar rejeição seca sem motivo declarado.** "Refaz", "não é isso", "não gostei" isolados são ruído. Exceção: se a mesma rejeição sem motivo recorrer em ≥3 sessões na mesma área, o padrão em si é sinal — registrar como hipótese com contador, nunca como faceta confirmada.
4. **Citação literal curta > paráfrase**, quando a evidência for mostrada ao dono. Máximo ~15 palavras; cortar com "[...]" se o trecho for maior.
5. **Nunca converter aceitação silenciosa em preferência declarada** na hora de aplicar o rascunho ou a atualização. Esta disciplina precisa sobreviver ao uso do arquivo, não só à sua construção.

## Contradições

Três casos, tratamento distinto:

**Mesma anomalia encontrada independentemente → condição de contorno, não contradição.** Se duas sessões sinalizam a mesma exceção no mesmo tipo de situação, a consistência é informativa — a regra existe mas foi escrita larga demais. Sugerir a reformulação ao dono, nunca aplicá-la sozinho.

**Conflito aparente que dissolve com leitura fina.** Duas leituras que parecem opostas podem ambas ser verdadeiras em níveis diferentes (uma descreve forma, outra substância). Checar antes de declarar conflito.

**Conflito genuíno.** Apresentar as duas versões e a evidência de cada uma, lado a lado. **Não resolver por conta própria** escolhendo a mais recente, mais frequente ou mais conveniente — isso é exatamente o que o modo avulso pede para o dono decidir (ver `SKILL.md`, passo 5 do fluxo avulso). Um rascunho que adivinha é pior que um que admite lacuna: o palpite fica invisível depois que a linha é escrita.

## Merge entre passadas independentes

Quando duas leituras alimentam a mesma faceta — por exemplo, a fonte A (Claude Code) e a fonte B (export do claude.ai) na mesma chamada, ou duas passadas avulsas em momentos diferentes:

- `●` sinal confirmado nas duas → firme, evidência mais forte que este método produz.
- `○` só numa fonte → válido, amostra estreita — aplicar com atenção contextual, nunca com o mesmo peso de `●`.
- `⚠` contradição entre as duas → registrar para o dono, nunca resolver sozinho.

Sinal suportado só por `[ACEITO]` nunca recebe `●`, mesmo aparecendo nas duas fontes. Dois sinais fracos continuam fracos — convergência não promove tag.
