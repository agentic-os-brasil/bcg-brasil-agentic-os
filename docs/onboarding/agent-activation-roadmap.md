# Roadmap de ativação dos agentes

O Maestro já pode abrir um workspace, mas o primeiro valor profissional aparece
quando você cria os agentes que representam o trabalho real. Pense nesta ordem:

```text
Maestro → Client Account Agent → Case Agent → primeira entrega → continuidade
```

O scaffold criado automaticamente pelo `bcgos init` é apenas a base técnica do
workspace. Ele não substitui a criação de um Client Account Agent para uma
conta nem de um Case Agent para um projeto concreto.

## A jornada recomendada

### 1. Crie o Client Account Agent da conta

Faça isso para cada cliente ou conta que tenha relacionamento recorrente,
contexto próprio e mais de um caso ao longo do tempo.

Exemplo:

> “Quero criar o Client Account Agent da Aurora Mobility. Ele deve organizar o
> relacionamento, histórico autorizado, stakeholders e prioridades da conta.
> O agente não deve acessar outros clientes nem inventar informações que não
> estejam nas fontes aprovadas.”

O resultado esperado é um agente com identidade, escopo de conta, mandato
limitado e fontes autorizadas. O Client Account Agent prepara e valida o
contexto; ele não executa o caso no lugar do Case Agent.

### 2. Crie o Case Agent de cada projeto

Um caso é um projeto, workstream ou entrega com objetivo e critérios próprios.
Crie um Case Agent por caso relevante e vincule-o ao Client Account Agent
correspondente.

Exemplo:

> “Agora crie o Case Agent do projeto de pricing 2026 da Aurora Mobility,
> vinculado à conta Aurora Mobility. O objetivo é preparar uma recomendação de
> pricing para a reunião de steering de setembro, usando somente as fontes
> aprovadas neste workspace.”

O resultado esperado é um agente com escopo de caso, objetivo, limites, fontes
e critério de sucesso. O Case Agent executa e entrega; não amplia o escopo da
conta, não cria acesso a outros casos e não fala diretamente com o cliente.

### 3. Dê ao caso uma primeira entrega pequena

Não crie agentes apenas para preencher cadastro. Dê ao Case Agent uma entrega
que possa ser concluída e revisada rapidamente:

> “Para começar, transforme as três fontes aprovadas em um briefing de uma
> página com fatos, hipóteses, lacunas e a próxima decisão necessária. Mostre o
> plano antes de produzir o briefing.”

Uma boa primeira entrega prova se a conta, o caso, as fontes e o critério de
qualidade estão bem definidos.

### 4. Confirme a continuidade

Depois da primeira entrega, peça ao Maestro:

> “Registre o checkpoint deste caso, indique a próxima ação e me mostre como
> retomar o trabalho na próxima sessão.”

O objetivo é que a próxima sessão encontre o caso certo, o estado certo e a
próxima ação certa — sem repetir o briefing inteiro.

## Checklist de ativação

- [ ] Maestro inicializado e onboarding do owner confirmado;
- [ ] Client Account Agent criado para a conta, quando houver contexto de cliente;
- [ ] Case Agent criado para o projeto/caso;
- [ ] Case Agent vinculado à conta correta;
- [ ] objetivo, fontes autorizadas, limites e critério de sucesso definidos;
- [ ] primeira entrega pequena revisada;
- [ ] checkpoint e próxima ação confirmados.

## Como saber o que falta

Se a pessoa disser apenas “quero trabalhar neste projeto”, o Maestro deve
impulsionar a próxima etapa com uma pergunta simples:

> “Antes de começar, vamos organizar o trabalho: este projeto pertence a qual
> conta? Posso criar o Client Account Agent e, em seguida, o Case Agent ligado a
> ele.”

Para um trabalho interno sem conta de cliente, o fluxo direto para Case Agent
continua válido. Para trabalho de cliente, a recomendação padrão é sempre
começar pela conta e depois criar o caso vinculado.

Listar agentes é apenas consulta; não cria nada. A criação deve ocorrer por uma
solicitação explícita do owner, com identidade e escopo revisados antes de
persistir.
