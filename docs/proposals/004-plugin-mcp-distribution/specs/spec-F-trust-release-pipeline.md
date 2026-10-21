# Spec F — Pasta compartilhada, publicação e recall

**DRAFT / F1 BLOCKED.** #438 depende de #436; rollout #439/#440.
Contrato de confiança existente: [Spec 020](../../../../specs/020-release-distribution.md).

## Canal padrão e limites

Marketplace local `directory` em pasta compartilhada somente leitura para
colegas; apenas publicador autorizado escreve. `.claude-plugin/marketplace.json`
usa fontes relativas para plugins em `versions/<versao>/`. Conteúdo, handlers
e helper nativo são versionados juntos; fonte do repositório não é distribuída.
Pasta concreta ainda não foi selecionada. Disponibilidade offline, sincronização
e políticas são condições a verificar, não garantias da abstração de diretório.

A expectativa é carregar a versão publicada na próxima sessão, sem depender
do autoupdater ou exigir `plugin update`. CLI 2.1.233 observou cache físico mas
selecionou origem publicada e releu um hook alterado numa nova inicialização.
Essa prova manteve versão/ponteiro iguais: #429/#438 ainda precisam observar
skill e troca N→N+1 no Code tab real, Mac/Windows e pasta sincronizada.

## Publicar: ponteiro por último

1. Construir ambos plugins por allowlist, a partir do mesmo source aprovado,
   com helper por plataforma em `scripts/`. Validar inventário e manifests.
2. Preparar versão nova fora do ponteiro ativo, copiar a árvore completa para
   `versions/<versao>/` e verificar hashes **no destino**. Falha preserva versão ativa.
3. Verificar autoridade e assinatura antes de promover. Versão existente só
   admite repetição byte-idêntica; bytes diferentes são recusados.
4. Só após essas verificações, substituir o marketplace por uma escrita local
   atômica que referencia essa versão. Nunca editar uma versão publicada.
5. Observar nova sessão, versão ativa e estado preservado. OneDrive pode
   sincronizar ponteiro e payload fora de ordem: troca local atômica não prova
   atomicidade entre máquinas; doctor deve detectar incompletude e bloquear PASS.

Reusar hashes/verificação existentes. A Spec 020 define Ed25519 e raiz local
aprovada independente do provedor, mas manifest v1 descreve CLI/base, não dois
plugins. #436/#438 devem fechar extensão versionada e compatibilidade antes
de chamar o pacote assinado. SHA-256 isolado não atesta autenticidade.
Chave privada não acompanha produto, pasta nem repositório. Doctor executado
depois do hook não é controle preventivo: verificação pré-execução precisa de
verificador já confiável e registro de chaves independente, ainda a fechar.
Nenhum plugin pode autenticar sua própria substituição de raiz de confiança.

## Recall

Um comando valida a versão anterior e reponta o marketplace por último;
mantém versões imutáveis e registra operação. Não modifica `~/.maestro/` nem
desinstala à distância. Provar no Code tab qual sessão passa a usar a versão
anterior; sessões abertas, cópias offline e atraso de sync não têm recall imediato.
Doctor informa versão/ponteiro e freshness; não inventar campo `revoked` no
schema de marketplace para obter uma capacidade não demonstrada.

## Política e plano B

#430 verifica tier padrão e controles de marketplace/hooks. Se política
restringir o canal, parar publicação por pasta e avaliar **claude.ai org library**
como plano B, com aprovação organizacional. Não é parte do caminho padrão da
v0.3.0 e não se altera managed settings ou `DISABLE_AUTOUPDATER=1`.
Assinatura/notarização corporativa, Gatekeeper/SmartScreen e clean-device são
gates próprios. A baseline 0.2.0 continua candidata unsigned/HOLD.

## Aceite

- [ ] Falha de cópia/hash/autoridade mantém ponteiro anterior; repetição é segura.
- [ ] Re-publicar versão com bytes diferentes falha; mutação de artefato é detectada.
- [ ] N→N+1 e recall observados em novas sessões Code tab Mac/Windows, sem
  update explícito, com snapshot controlado de estado byte-idêntico.
- [ ] Pasta somente leitura, sync parcial, offline, spaces e bloqueio nativo
  têm evidência e recuperação documentada, sem bypass.
- [ ] Autoridade de release e limites do recall são explícitos antes do piloto.
