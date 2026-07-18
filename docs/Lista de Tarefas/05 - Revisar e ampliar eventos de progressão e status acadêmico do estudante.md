---
criado: 2026-07-18 00:00
origem: Lista de tarefas.md
status: pendente
---

# Revisar e ampliar eventos de progressão e status acadêmico do estudante (pendente)

## Prompt recomendado para executar a atualização

Faça uma auditoria dos endpoints já implementados no escopo "Endpoints de acontecimentos que alteram status do estudante" (`POST /academia/estudante/:codigo/matricula/fundamental|medio|superior`, `interrupcao/fundamental|medio`, `trancamento/superior`, `desvincular`, `revincular`), corrigindo inconsistências de pré-condição entre eles, e implemente um novo evento de ajuste administrativo que permita à academia corrigir `ano_escolar_fundamental`, `ano_escolar_medio`, `ano_superior`/`semestre_atual` ou o curso vinculado do estudante fora do fluxo normal de matrícula/avaliação final, sempre com justificativa obrigatória e sempre gravado como evento auditável no ledger. Ao final, atualize testes, documentação técnica e qualquer documentação afetada. Não implemente atualização direta de campo sem evento correspondente — o padrão de event sourcing já estabelecido em `docs/Tarefas feitas/Atualização na definição dos status do estudante.md` deve ser preservado em qualquer nova capacidade.

## Contexto

`Documentação.md` (seção 8, "Endpoints de acontecimentos que alteram status do estudante") documenta um conjunto de rotas que alteram `status_escolar_fundamental`, `status_escolar_medio`, `status_superior` e os campos de posição acadêmica (`ano_escolar_fundamental`, `ano_escolar_medio`, `ano_superior`, `semestre_atual`, `curso_medio_id`, `curso_superior_id`) sempre como consequência de um evento de domínio real, e nunca por atualização direta de campo — esse princípio foi estabelecido deliberadamente em `docs/Tarefas feitas/Atualização na definição dos status do estudante.md`.

`Lista de tarefas.md` traz dois itens relacionados a essa mesma área que esta tarefa consolida:

1. "Melhorar as regras de negócio para as rotas/eventos que estão no escopo 'Endpoints de acontecimentos que alteram status do estudante' da documentação, e talvez os melhorar o funcionamento desses mesmos eventos.";
2. "Academia poder mudar o ano acadêmico/curso/semestre do estudante".

O segundo item é, na prática, uma lacuna concreta dentro do primeiro: hoje não existe **nenhum** caminho para a academia corrigir um erro de cadastro (por exemplo, `ano_escolar_fundamental` lançado errado na matrícula) ou reconhecer conclusão externa de uma etapa (por exemplo, um estudante transferido de outra instituição que já concluiu o ensino fundamental fora do Spuri) sem forçar o estudante por um fluxo que não corresponde ao fato real. Por isso, as duas tarefas foram combinadas: a revisão dos endpoints existentes (item 1) e a nova capacidade administrativa (item 2) tratam do mesmo domínio — a posição acadêmica do estudante — e compartilham as mesmas validações de curso/ano/nível já usadas pelos endpoints de matrícula.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Auditoria dos endpoints existentes | Revisar pré-condições de `matricula/*`, `interrupcao/*`, `trancamento/superior`, `desvincular`, `revincular` | Consistência de comportamento entre eles, sem sobreposição indevida |
| Reconhecimento de conclusão externa | Adicionar caminho para matricular estudante transferido sem exigir conclusão interna da etapa anterior | Academia registra evento de conclusão/equivalência externa antes de matricular no próximo nível |
| Correção de curso já ativo | Impedir que `matricula/medio` e `matricula/superior` troquem `curso_id` silenciosamente de um estudante já em andamento | Mudança de curso continua exclusiva do fluxo já existente de alteração de curso |
| Ajuste administrativo de posição acadêmica | Novo evento auditável para correção pontual de ano/curso/semestre | Academia corrige erro de cadastro sem contornar o sistema com desvínculo/revínculo |

---

# 1. Auditar consistência dos endpoints de acontecimentos já existentes

## Objetivo

Confirmar que os endpoints já implementados no escopo "Endpoints de acontecimentos que alteram status do estudante" têm pré-condições coerentes entre si, sem permitir efeitos colaterais não documentados.

## Escopo mínimo de investigação

Investigar, no mínimo, os handlers, aggregates e eventos de:

- `POST /academia/estudante/:codigo/matricula/fundamental` (`MatriculaFundamentalEfetivada`);
- `POST /academia/estudante/:codigo/matricula/medio` (`MatriculaMedioEfetivada`);
- `POST /academia/estudante/:codigo/matricula/superior` (`MatriculaSuperiorEfetivada`);
- `POST /academia/estudante/:codigo/interrupcao/fundamental` (`FundamentalInterrompido`);
- `POST /academia/estudante/:codigo/interrupcao/medio` (`MedioInterrompido`);
- `POST /academia/estudante/:codigo/trancamento/superior` (`SuperiorTrancado`);
- `POST /academia/estudante/:codigo/desvincular` (`EstudanteDesvinculadoDaAcademia`);
- `POST /academia/estudante/:codigo/revincular` (`EstudanteReintegrado`);
- o evento `CursoAlterado`, já existente conforme `docs/Tarefas feitas/Preservar histórico acadêmico ao desvincular ou inativar estudante.md`, e sua relação com `matricula/medio`/`matricula/superior`.

## Checklist obrigatório de auditoria

### 1.1 `matricula/medio` e `matricula/superior` não devem contornar `CursoAlterado`

Confirmar se, hoje, chamar `matricula/medio` ou `matricula/superior` com um `curso_id` **diferente** do curso atual, enquanto `status_escolar_medio`/`status_superior` já está `em_andamento`, troca o curso do estudante silenciosamente. Se confirmado, corrigir para que:

- `matricula/medio`/`matricula/superior` só possam ser usadas para: (a) primeira matrícula no nível, ou (b) retomada após interrupção/trancamento no **mesmo** curso já registrado;
- uma tentativa de informar um `curso_id` diferente enquanto o nível já está `em_andamento` seja rejeitada com erro claro, orientando o uso do fluxo já existente de alteração de curso (`CursoAlterado`).

### 1.2 Retomada após interrupção não deve regredir a posição acadêmica

Confirmar se `matricula/fundamental` e `matricula/medio`, ao serem chamadas novamente após uma interrupção (`status_escolar_fundamental`/`status_escolar_medio = inativo`), validam que o `ano_escolar_fundamental`/`ano_escolar_medio` informado é compatível com a posição anterior do estudante (preservando o princípio já estabelecido em `docs/Tarefas feitas/Preservar histórico acadêmico ao desvincular ou inativar estudante.md` de que reativação não deve resetar progressão). Se a validação não existir, adicioná-la: a retomada não deve aceitar um ano **anterior** ao que o estudante já havia alcançado, salvo se acompanhada de justificativa explícita via o novo evento de ajuste administrativo descrito na seção 3.

### 1.3 Reconhecimento de conclusão/equivalência externa

Hoje, `matricula/medio` exige `status_escolar_fundamental = "finalizado"`, e esse status só é alcançável internamente via aprovação na avaliação final do último ano fundamental dentro do próprio Spuri. Isso impede que a academia matricule diretamente no médio um estudante transferido de outra instituição que já concluiu o fundamental fora da plataforma.

Implementar um evento de reconhecimento de conclusão externa por nível — por exemplo `ConclusaoFundamentalExternaReconhecida` e `ConclusaoMedioExternaReconhecida` — com as seguintes regras:

1. exige `motivo`/documento de referência (reaproveitando o padrão de upload de documento comprobatório já usado em outras partes do sistema, quando aplicável);
2. define `status_escolar_fundamental`/`status_escolar_medio = "finalizado"` sem exigir progressão interna prévia;
3. não cria nem altera notas, faltas ou avaliações finais — é estritamente um reconhecimento de status, auditável separadamente de uma avaliação final real;
4. depois de registrado, libera o uso normal de `matricula/medio`/`matricula/superior` como já documentado, sem nenhuma mudança adicional nesses endpoints.

### 1.4 Consistência de `motivo` obrigatório

Confirmar que todas as rotas que hoje exigem `motivo` (`interrupcao/fundamental`, `interrupcao/medio`, `trancamento/superior`, `desvincular`) realmente rejeitam motivo vazio/composto só de espaços após `trim`, com a mesma mensagem de erro consistente entre elas.

### 1.5 Testes de regressão da auditoria

1. `matricula/medio` com `curso_id` diferente do atual, estudante já `em_andamento`: rejeitado, orientando `CursoAlterado`;
2. `matricula/superior` com `curso_id` diferente do atual, estudante já `em_andamento`: rejeitado, orientando `CursoAlterado`;
3. `matricula/fundamental` retomando após interrupção com ano anterior ao já alcançado: rejeitado;
4. reconhecimento de conclusão externa do fundamental seguido de `matricula/medio`: sucesso, sem exigir avaliação final interna;
5. reconhecimento de conclusão externa do médio seguido de `matricula/superior`: sucesso;
6. `motivo` vazio ou só com espaços rejeitado de forma consistente em todas as rotas que já exigem motivo.

---

# 2. Criar evento de ajuste administrativo de posição acadêmica

## Objetivo

Permitir que a academia corrija `ano_escolar_fundamental`, `ano_escolar_medio`, `ano_superior`/`semestre_atual` ou o curso vinculado de um estudante já em andamento, para casos de erro de cadastro ou reconhecimento de equivalência que não se encaixam nos eventos já existentes, sempre com justificativa obrigatória e sempre auditável.

## Regra de negócio

Criar `POST /academia/estudante/:codigo/ajuste-academico`, protegido por autenticação de academia ativa, com o seguinte contrato conceitual:

```json
{
  "nivel": "fundamental",
  "ano_escolar_fundamental": "5_ano_fundamental",
  "categoria_motivo": "correcao_cadastral",
  "motivo": "ano academico informado incorretamente na matricula inicial"
}
```

```json
{
  "nivel": "superior",
  "curso_id": "uuid-do-curso-superior",
  "semestre_atual": 3,
  "categoria_motivo": "equivalencia_externa",
  "motivo": "estudante transferido com aproveitamento de disciplinas cursadas em outra instituicao"
}
```

O backend deve:

1. exigir que o `nivel` informado (`fundamental`, `medio` ou `superior`) esteja atualmente `em_andamento` para o estudante — este endpoint **não** inicia um nível novo; ele apenas corrige a posição de um nível já ativo. Iniciar um nível continua sendo responsabilidade exclusiva de `matricula/*`;
2. validar o(s) campo(s) informado(s) com exatamente as mesmas regras já usadas em `matricula/medio`/`matricula/superior` para curso/ano/semestre (curso existe, está ativo, pertence à academia, é do tipo correto; ano/semestre é compatível com o curso quando aplicável);
3. para `superior`, se `semestre_atual` for informado, recalcular `ano_superior = ceil(semestre_atual / 2)` automaticamente, nunca aceitando os dois de forma inconsistente entre si;
4. exigir `motivo` não vazio e `categoria_motivo` dentre um conjunto fechado de valores (ex.: `correcao_cadastral`, `equivalencia_externa`, `outro`), para permitir relatórios administrativos futuros sem depender de texto livre;
5. emitir um evento auditável, por exemplo `AjustePosicaoAcademicaRealizado`, registrando os valores anteriores e novos de cada campo alterado, o `motivo`, a `categoria_motivo` e o usuário/academia executante;
6. **não** alterar `status_escolar_fundamental`, `status_escolar_medio` nem `status_superior` — este endpoint corrige apenas posição (ano/curso/semestre), nunca o status do vínculo.

## Escopo obrigatório

### 2.1 Não substituir avaliação final

Este endpoint não deve ser usado, nem permitir uso, como substituto da avaliação final automática. A documentação e as mensagens de erro devem deixar claro que ele serve apenas para correção administrativa pontual, e que progressão acadêmica normal continua sendo derivada exclusivamente da avaliação final.

### 2.2 Auditoria e consulta

O evento `AjustePosicaoAcademicaRealizado` deve aparecer em `GET /eventos-estudante/:codigo`, com os valores anteriores e novos claramente legíveis, permitindo auditoria completa de quando e por que a posição do estudante foi corrigida administrativamente.

### 2.3 Testes obrigatórios

1. ajuste de `ano_escolar_fundamental` de um estudante com fundamental `em_andamento`: sucesso, evento auditável com valores antes/depois;
2. ajuste de `semestre_atual` de um estudante superior: `ano_superior` é recalculado automaticamente e de forma consistente;
3. tentativa de ajuste com o nível **não** `em_andamento` (ex.: fundamental `inativo`): rejeitado, orientando o uso de `matricula/fundamental`;
4. tentativa de ajuste sem `motivo` ou com `motivo` vazio: rejeitado;
5. tentativa de ajuste com `categoria_motivo` fora do conjunto permitido: rejeitado;
6. ajuste de curso via este endpoint segue exatamente as mesmas validações de curso já usadas em `matricula/medio`/`matricula/superior`;
7. este endpoint nunca altera `status_escolar_fundamental`, `status_escolar_medio` ou `status_superior`.

---

# 3. Atualização obrigatória da documentação

Atualizar `Documentação.md`, seção 8 ("Endpoints de acontecimentos que alteram status do estudante"), incluindo:

- os dois novos eventos de conclusão/equivalência externa (`ConclusaoFundamentalExternaReconhecida`, `ConclusaoMedioExternaReconhecida`);
- o novo endpoint `POST /academia/estudante/:codigo/ajuste-academico` e o evento `AjustePosicaoAcademicaRealizado`;
- a regra revisada de `matricula/medio`/`matricula/superior` não trocar curso silenciosamente;
- a regra revisada de retomada não regredir posição acadêmica sem justificativa.

---

# Fora de escopo

- Alterar a lógica de cálculo da avaliação final automática.
- Alterar `CursoAlterado` além do necessário para deixar clara sua relação com `matricula/medio`/`matricula/superior`.
- Criar interface administrativa (UI) para o ajuste acadêmico; o escopo é backend.
- Permitir que o ajuste administrativo altere status de vínculo (`ativo`/`arquivado`) ou status de nível (`inativo`/`em_andamento`/`finalizado`).
- Criar aliases, wrappers de compatibilidade ou caminhos alternativos que dupliquem a validação já existente em `matricula/medio`/`matricula/superior`.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `matricula/medio` e `matricula/superior` não trocarem curso silenciosamente de um estudante já `em_andamento`;
2. a retomada após interrupção não permitir regressão de posição acadêmica sem justificativa;
3. existir caminho auditável para reconhecer conclusão/equivalência externa de fundamental e médio;
4. `POST /academia/estudante/:codigo/ajuste-academico` existir, com as validações e o evento auditável descritos na seção 2;
5. o novo endpoint nunca alterar status de vínculo ou de nível, apenas posição acadêmica;
6. `Documentação.md` estar atualizada com os novos eventos e endpoints;
7. testes automatizados cobrirem os cenários das seções 1.5 e 2.3;
8. o PR explicar claramente as inconsistências corrigidas e a nova capacidade administrativa.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Revisar e ampliar eventos de progressão e status acadêmico do estudante (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
