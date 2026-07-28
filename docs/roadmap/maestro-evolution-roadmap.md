# Roadmap de evolução do Maestro

Este é o roadmap de produto: explica para onde o Maestro pode evoluir, qual
valor cada etapa libera e qual evidência precisa existir antes de ampliar a
promessa. O [`ROADMAP.md`](../../ROADMAP.md) continua sendo o backlog técnico
detalhado; este documento é a ponte entre arquitetura, adoção e valor para o
negócio.

## Norte do produto

Transformar trabalho profissional fragmentado em um sistema local, contínuo e
governado: o usuário define o resultado, o Maestro preserva o contexto
necessário, orienta a próxima ação, registra evidência e melhora com sinais
estruturais — sem capturar indiscriminadamente o conteúdo de cliente.

O Maestro não deve competir por ser “o agente que faz tudo”. Sua vantagem é
organizar o trabalho que precisa continuar, com uma fronteira de autoridade que
uma organização consegue explicar, testar e auditar.

## Como vender a proposta sem exagerar

### A frase de valor

**Maestro é a camada de continuidade e governança para trabalho profissional
assistido por agentes.**

### As cinco vantagens que importam

1. **Menos recomeço:** sessões, pessoas e agentes retomam de um estado explícito,
   com objetivo, checkpoint e próxima ação.
2. **Mais confiança:** ações críticas têm escopo, identidade, evidência e
   revisão; o sistema falha fechado quando a autoridade não está provada.
3. **Privacidade operacional:** o produto melhora a operação a partir de sinais
   tipados, não da exportação de prompts, documentos ou dados de cliente.
4. **Adoção sem dependência técnica:** a jornada de piloto é pensada para
   `bcgos`, não para Git, Go, Python, Docker ou configuração manual de modelos.
5. **Portabilidade governada:** Claude e Codex podem projetar a mesma política
   sem transformar uma implementação específica em contrato do produto.

### Para quem o produto é especialmente valioso

| Perfil | Dor recorrente | Ganho esperado |
| --- | --- | --- |
| Consultor clássico | Handoffs, fontes espalhadas e decisões que perdem contexto. | Briefings, checkpoints e decisões retomáveis. |
| Consultor técnico | Muitas ferramentas, artefatos e validações em paralelo. | Ledger, evidências e quality loops no mesmo fluxo. |
| Data scientist/engineer | Trabalho longo, dependências e necessidade de reproduzir o caminho. | Contratos, ingestão local, proveniência e recuperação. |
| Líder de equipe | Dificuldade de saber o que foi feito e com qual risco. | Receipts seguros, critérios e estado legível sem coletar conteúdo. |
| Responsável por segurança | Agentes com escopo incerto e telemetria excessiva. | Fail-closed, raízes separadas e autorização explícita. |

## Ponto de partida atual

O repositório está no nível **contract-validated** da escada de maturidade:

1. contratos, schemas, CLI local e harness têm implementação e testes;
2. runtime nativo e adapters ainda precisam de evidência qualificadora;
3. distribuição assinada, instalação em dispositivo limpo, update/rollback e
   suporte de piloto ainda são gates separados;
4. ingestão tem Docling como rota primária definida e MarkItDown como fallback
   delimitado, mas a conversão permanece indisponível sem runtime pack aprovado.

Isso é uma base sólida para evoluir, mas não deve ser descrito como piloto de
produção. Cada horizonte abaixo só libera sua mensagem comercial depois do gate
correspondente.

## Horizontes de evolução

### Horizonte 0 — Tornar o primeiro valor repetível

**Resultado:** uma pessoa consegue configurar um workspace limpo, completar um
caso de uso pequeno, pausar e retomar sem depender do time de engenharia.

**Evoluções possíveis:**

- escolher e aprovar um único caso de uso inicial para perfis clássicos e
  técnicos;
- transformar `bcgos init`, `status` e `doctor` em um percurso guiado;
- concluir onboarding Windows e macOS em dispositivos corporativos;
- tornar o `maestro-setup-update` o concierge de instalação, recuperação e
  rollback;
- definir suporte, incidente, sucesso e critérios de parada do piloto.

**Evidência para avançar:** clean-device acceptance, tempo até primeiro sucesso,
retomada reproduzível e ausência de mistura entre workspace e core.

**Valor vendido:** “o sistema começa pequeno, explica o que está acontecendo e
não exige que o consultor vire engenheiro de plataforma”.

### Horizonte 1 — Runtime e distribuição confiáveis

**Resultado:** o produto chega ao dispositivo certo, na versão certa, e pode ser
atualizado ou revertido sem intervenção artesanal.

**Evoluções possíveis:**

- artefatos privados versionados, assinados e autenticados;
- bootstrapper estável com verificação de manifest, compatibilidade e
  proveniência;
- instalação, ativação, update, rollback e remoção idempotentes;
- packs separados para componentes pesados, com preflight de disco, proxy,
  CPU e offline;
- paridade Windows/macOS e suporte operacional com dono definido.

**Evidência para avançar:** assinatura de produção, instalação em dispositivos
limpos, update/rollback aceitos e diagnóstico útil para usuário não técnico.

**Valor vendido:** “a organização controla o ciclo de vida do agente como
controla software corporativo, sem pedir clone, `pip` ou binário baixado à mão”.

### Horizonte 2 — Primeiro caso de uso e ingestão local

**Resultado:** um fluxo profissional completo transforma fontes autorizadas em
conhecimento local revisável e gera uma decisão ou artefato útil.

**Evoluções possíveis:**

- Docling local como rota principal para documentos elegíveis;
- MarkItDown como fallback determinístico para formatos aprovados;
- validação de estrutura, Markdown, proveniência, tamanho e fidelidade;
- fixtures sanitizadas e medição de tabelas, OCR, imagens e documentos Office;
- batch local, templates de extração e exportações intermediárias para perfil
  `advanced`/`power`;
- integração segura com `ingest-content`, análise qualitativa/quantitativa e
  `deck-storyline`;
- rejeição explícita de fallback remoto implícito.

**Evidência para avançar:** runtime pack verificado em Windows/macOS, tamanho e
first-use aceitáveis, comportamento offline conhecido e qualidade aprovada no
caso de uso selecionado.

**Valor vendido:** “seus documentos viram material de trabalho estruturado no
ambiente autorizado, com rota e qualidade visíveis — não uma caixa-preta de
upload”.

### Horizonte 3 — Trabalho contínuo e agentes governados

**Resultado:** o Maestro coordena tarefas longas e handoffs entre sessões,
agentes e runtimes sem transformar delegação em acesso irrestrito.

**Evoluções possíveis:**

- lifecycle nativo qualificado para Claude e Codex;
- identidade de agente vinculada a capacidade, escopo, ferramenta e recurso;
- branch/recoverable state para impedir execuções paralelas não autorizadas;
- receipts metadata-only ligados ao ledger;
- Walter como gate autenticado de conclusão de trabalho de maior criticidade;
- handoff entre duas sessões/runtimes com ponteiros, não transcript completo;
- catálogo de agentes e skills com ownership, versão, compatibilidade e
  retirement.

**Evidência para avançar:** matriz de lifecycle com sessão nativa qualificada,
conformance Claude/Codex, recuperação após interrupção e revisão humana
demonstrável.

**Valor vendido:** “o agente não apenas responde; ele trabalha dentro de um
  sistema de responsabilidade que alguém consegue inspecionar”.

### Horizonte 4 — Memória profissional e conhecimento navegável

**Resultado:** o usuário encontra padrões e decisões anteriores sem criar um
  depósito invisível ou irrevogável de tudo que já foi dito.

**Evoluções possíveis:**

- L1 diário, L2 semanal, L3 temático e lifetime curado;
- dreaming determinístico primeiro, com staging, validação e last-known-good;
- explicação, correção, exportação, retenção e deleção por camada;
- atlas/wiki derivado com backlinks, órfãos, broken links e manifest atômico;
- fronteira explícita entre produto gerenciado, owner-private e workspace-private;
- revogação e invalidação síncronas para conteúdo corrigido ou removido;
- conhecimento organizacional compartilhado como store governado separado, se
  houver aprovação.

**Evidência para avançar:** política de sinais, consentimento, retenção,
correção/remoção, provider/offline aprovados e testes de revogação.

**Valor vendido:** “o Maestro aprende o modo de trabalhar sem transformar a
  vida profissional em uma caixa-preta impossível de corrigir”.

### Horizonte 5 — Escala seletiva e ecossistema

**Resultado:** novas equipes, skills e integrações podem ser adicionadas sem
  dissolver a fronteira de segurança do núcleo.

**Evoluções possíveis:**

- bundles opcionais verificados por identidade e compatibilidade;
- SDK de extensão com manifest, capabilities e lifecycle fixtures;
- catálogo privado/marketplace controlado, com ownership e expiração;
- integrações corporativas aprovadas para identidade, distribuição e suporte;
- interface gráfica opcional para inspeção de ledger, atlas e diagnósticos;
- mais runtimes e Linux somente quando a paridade de contrato for demonstrada;
- métricas comparáveis de time-to-value, resume success, update reliability,
  intervenção e qualidade percebida.

**Evidência para avançar:** piloto controlado bem-sucedido, modelo de suporte,
retirement/rollback e revisão de segurança por extensão.

**Valor vendido:** “crescer não significa abrir um plugin irrestrito; significa
compor capacidades verificadas ao redor de um núcleo estável”.

## Dependências e ordem recomendada

```text
primeiro valor repetível
        ↓
distribuição e runtime confiáveis
        ↓
caso de uso + ingestão local
        ↓
agentes/lifecycle governados
        ↓
memória e atlas revogáveis
        ↓
extensões e escala
```

Não faz sentido prometer marketplace antes de haver release verificável; nem
prometer memória inteligente antes de haver direitos de inspeção, correção e
deleção. A ordem reduz risco de adoção e torna cada investimento uma prova do
próximo.

## Métricas para decidir evolução

O canary deve trabalhar com sinais mínimos e metadata-only. Para cada horizonte,
acompanhe:

- **time to first success:** quanto leva para completar a primeira entrega útil;
- **resume success:** quantas retomadas chegam ao próximo passo correto;
- **quality/fidelity:** qualidade percebida e fidelidade de ingestão por tipo;
- **install/update/rollback:** sucesso e tempo sem suporte artesanal;
- **manual interventions:** onde o produto ainda interrompe o usuário;
- **capability failures:** quais capacidades permanecem bloqueadas ou indisponíveis;
- **support demand:** volume e repetição de incidentes;
- **safety evidence:** violações de escopo, falhas de assinatura, vazamentos e
  reversibilidade.

Métricas não devem conter prompts, documentos, nomes de cliente, caminhos
privados, credenciais ou transcripts. O número só vale quando a definição,
janela e limite de retenção estiverem registrados.

## O que não entra agora

Até que os gates anteriores estejam fechados, o Maestro não deve prometer:

- autonomia irrestrita ou execução silenciosa em nome do usuário;
- integração nativa apenas porque um hook local foi configurado;
- ingestão remota automática quando o caminho local falhar;
- memória eterna, perfeita ou impossível de apagar;
- produção em dispositivo não qualificado;
- marketplace, SDK público ou GUI como substitutos para contratos do núcleo.

## Como este roadmap é governado

Cada evolução deve registrar: problema, persona, contrato afetado, dados
autorizados, mudança de autoridade, evidência de aceitação, owner, rollback e
impacto na comunicação do produto. Questões sem decisão permanecem em
[`docs/OPEN-QUESTIONS.md`](../OPEN-QUESTIONS.md); decisões aceitas entram no
decision log; o backlog técnico detalha a implementação.

## Assinatura do projeto

Maestro — BCG Brasil Agentic OS

Desenvolvido por:

- Daniel Scardini
- Julia Ribeiro
- Marcelho Sanches
