---
criado: 2026-06-28 00:00
origem: pedido_usuario_chatgpt_2026-06-28
status: pronto_para_implementacao
modificado: 2026-06-28 00:00
---

# Manter ensino fundamental em andamento quando aprovado sem próximo ano acadêmico na academia

## Prompt recomendado para executar a atualização

Implemente no backend uma regra de negócio para avaliações finais do ensino fundamental: quando um estudante do fundamental for aprovado, mas a academia não tiver o próximo ano acadêmico do fundamental habilitado/ofertado, o status do fundamental **não pode** ser definido como `finalizado`. O estudante deve permanecer com `status_escolar_fundamental = "em_andamento"`, continuar vinculado à mesma academia, ter seu `ano_escolar_fundamental` atualizado para o próximo ano acadêmico global e não ser automaticamente vinculado a nenhuma turma, matéria, curso, período ou outro agrupamento desse ano ainda não ofertado pela academia. O sistema deve registrar a aprovação normalmente, preservar histórico, evitar conclusão indevida do ciclo fundamental e deixar o estudante em estado pendente/aguardando oferta do próximo ano acadêmico.

## Contexto do problema

Hoje a avaliação final calcula `proximo_ano_academico` para o fundamental pela sequência fixa `1_ano_fundamental` até `9_ano_fundamental`. Quando o estudante é aprovado e não há próximo ano na sequência, a ausência de `proximo_ano_academico` representa o fim real do ciclo e pode finalizar o fundamental. Porém existe outro cenário: a sequência global possui próximo ano, mas a academia atual não oferece/habilitou esse próximo ano. Nesse caso, a ausência operacional de vínculo não significa conclusão do ensino fundamental; significa apenas que a academia não tem o ano seguinte configurado.

Exemplo:

```json
{
  "codigo_estudante": "EST-001",
  "codigo_academia": "ACAD-001",
  "tipo_ensino": "fundamental",
  "ano_academico_atual": "5_ano_fundamental",
  "aprovado": true,
  "proximo_ano_global": "6_ano_fundamental",
  "anos_academicos_da_academia": ["1_ano_fundamental", "2_ano_fundamental", "3_ano_fundamental", "4_ano_fundamental", "5_ano_fundamental"]
}
```

Resultado esperado:

```json
{
  "status_escolar_fundamental": "em_andamento",
  "ano_escolar_fundamental": "6_ano_fundamental",
  "codigo_academia": "ACAD-001",
  "turma_id": null,
  "proximo_ano_academico": "6_ano_fundamental",
  "motivo": "academia_sem_oferta_do_proximo_ano_academico_fundamental"
}
```

## Estado atual observado no código

A implementação deve confirmar estes pontos no código antes de editar:

- O handler de avaliação final calcula aprovação, `proximo_ano_academico` e registra o evento `AvaliacaoFinalAnoAcademico`.
- O fundamental usa uma sequência fixa em `calcularProximoAnoFundamental`, que retorna `nil` quando o estudante aprovado está no último ano da sequência global.
- O processamento/projeção da avaliação final tende a interpretar aprovação sem próximo ano como conclusão do ciclo, atualizando `status_escolar_fundamental` para `finalizado`.
- Academias fundamental/mistas possuem lista de `anos_academicos` na própria academia; essa lista deve ser consultada para saber se o próximo ano fundamental está habilitado/ofertado na academia atual.
- A regra desta tarefa é diferente da finalização real do `9_ano_fundamental`: se o estudante concluiu o último ano global do fundamental, pode finalizar; se ainda existe próximo ano global, mas a academia não oferta esse ano, deve continuar `em_andamento`.

## Decisão de contrato

1. **Aprovação com próximo ano global ofertado pela academia**: promover normalmente para o próximo ano acadêmico, mantendo `status_escolar_fundamental = "em_andamento"` e realizando apenas os vínculos automáticos já suportados pelo sistema.
2. **Aprovação no último ano global do fundamental**: finalizar o ciclo fundamental conforme regra existente, desde que o ano atual seja de fato o último da sequência global canônica.
3. **Aprovação com próximo ano global não ofertado pela academia**: não finalizar o fundamental; manter o estudante `em_andamento`, na mesma academia, atualizar `ano_escolar_fundamental` para o próximo ano global e deixar sem turma/matéria/vínculo automático enquanto a academia ainda não oferta esse ano.
4. **Reprovação**: manter comportamento atual de reprovação, sem promover e sem finalizar por causa desta regra.
5. **Ensino médio e superior**: não alterar comportamento nesta tarefa, exceto se algum helper compartilhado exigir adaptação segura sem mudar contrato.

## Objetivos funcionais

### 1. Diferenciar fim real do fundamental de ausência de oferta da academia

Ao processar uma aprovação do fundamental, o backend deve distinguir:

- `ano_atual == 9_ano_fundamental` ou último ano canônico configurado globalmente: ciclo fundamental pode ser finalizado.
- `ano_atual < último ano global`, mas `proximo_ano_global` não existe em `projection_academias.anos_academicos`: ciclo fundamental **não** pode ser finalizado.

Sugestão de helper:

```go
func resolverProgressaoFundamentalNaAcademia(
    anoAtual string,
    aprovado bool,
    anosAcademicosAcademia []string,
) (resultado ProgressaoFundamentalResultado, err error)
```

Resultado conceitual:

```go
type ProgressaoFundamentalResultado struct {
    ProximoAnoAcademico *string
    FinalizaCiclo       bool
    MantemEmAndamento   bool
    MotivoBloqueio      string
}
```

### 2. Promover para o próximo ano global mantendo estudante em andamento e sem vínculos automáticos

Quando o estudante aprovado ainda tiver próximo ano global, mas a academia não ofertar esse ano:

- manter `status_escolar_fundamental = "em_andamento"`;
- atualizar `ano_escolar_fundamental` para o próximo ano acadêmico global calculado, mesmo que esse ano ainda não exista na lista de anos ofertados pela academia;
- manter o vínculo com a mesma academia (`codigo_academia`/`academia_id` atual);
- não preencher turma automaticamente;
- não vincular matérias, curso, período ou turma para o próximo ano ainda não ofertado pela academia;
- não alterar para `finalizado`;
- registrar motivo técnico/auditável para facilitar suporte e relatórios.

### 3. Não usar falta de oferta da academia como motivo para zerar o próximo ano acadêmico

Para fundamental, `proximo_ano_academico` só deve ficar `nil/null` quando o ciclo for realmente concluído, por exemplo aprovação no último ano global canônico. Quando ainda existir próximo ano global, mas esse ano não estiver habilitado na academia atual, o backend deve manter `proximo_ano_academico` preenchido com o próximo ano global e apenas bloquear vínculos automáticos dependentes da oferta da academia.

O processamento do evento/projeção não deve finalizar o fundamental nem deixar o estudante no ano anterior nesse cenário.

### 4. Registrar resultado de avaliação sem criar vínculo inválido

A avaliação final deve continuar sendo registrada como aprovada. A regra não deve apagar ou impedir o resultado acadêmico. O que muda é a progressão/vínculo posterior:

- `aprovado = true` continua salvo no histórico de avaliação final;
- `proximo_ano_academico` deve indicar o próximo ano global calculado quando o estudante for aprovado e ainda houver próximo ano na sequência fundamental;
- incluir metadado/motivo, se o modelo de evento permitir, como `sem_oferta_do_proximo_ano_academico_na_academia = true` ou `motivo_progressao = "academia_sem_oferta_do_proximo_ano_academico_fundamental"`;
- não emitir evento de finalização do fundamental nesse cenário;
- não gerar vínculo automático para turma inexistente.

### 5. Compatibilizar com academias mistas

Para academia `misto`, a validação deve olhar apenas os anos acadêmicos do fundamental na configuração da academia. Não confundir com anos do ensino médio, cursos médios ou `anos_academicos` de cursos.

Regras:

- `6_ano_fundamental` deve ser buscado na lista de anos fundamentais da academia.
- `1_ano_medio` não deve ser considerado próximo ano válido do fundamental.
- Se a academia mista oferece fundamental até o `9_ano_fundamental`, o ciclo pode finalizar normalmente ao aprovar o 9º ano.

## Áreas prováveis de alteração

Use esta lista como guia inicial; confirme no código antes de editar.

- Handler de avaliação final:
  - `internal/handlers/avaliacao_final_handler.go`.
- Aggregate/eventos de estudante ou avaliação final:
  - arquivos em `internal/domain/aggregates/` relacionados a estudante e avaliação final.
- Projeções de estudante e avaliação final:
  - arquivos em `internal/projections/` relacionados a estudantes e avaliação final.
- Helpers de validação de anos acadêmicos:
  - `internal/utils/` ou helpers já existentes para anos do fundamental.
- Testes de handlers/projeções:
  - `internal/handlers/*avaliacao*test.go`.
  - `internal/projections/*estudante*test.go` ou equivalentes.
- Documentação da API, se a resposta de avaliação final expuser novo motivo/metadado:
  - `docs/Spuri - API.md`.

## Validações obrigatórias

### Progressão fundamental

- Validar que o `ano_academico_atual` pertence à sequência canônica do fundamental.
- Calcular o próximo ano global antes de consultar a academia.
- Consultar a academia atual do estudante, não uma academia enviada pelo cliente.
- Verificar se o próximo ano global está habilitado em `anos_academicos` da academia.
- Só finalizar quando não existir próximo ano global.
- Não finalizar quando existir próximo ano global, mas a academia não o ofertar.

### Segurança e consistência

- Não permitir que o frontend force `finalizado` nesse fluxo.
- Não gravar `proximo_ano_academico = null` apenas porque a academia não oferta o próximo ano; nesse caso, manter o próximo ano global calculado.
- Não transferir estudante automaticamente para outra academia.
- Não criar turma, matrícula, matéria, curso ou vínculo fictício para contornar a ausência do próximo ano.
- Preservar histórico e ledger; esta regra não deve recalcular cobranças, notas, faltas ou avaliações antigas.
- Garantir comportamento consistente em rebuild de projeções.

### Compatibilidade

- Não alterar a regra de aprovação/reprovação em si.
- Não alterar o comportamento do ensino médio e superior.
- Não quebrar o caso atual de conclusão real do `9_ano_fundamental`.
- Não confundir ano letivo/calendário com ano acadêmico do fundamental.

## Estratégia de implementação sugerida

1. Mapear onde a avaliação final grava evento e onde a projeção altera `status_escolar_fundamental` para `finalizado`.
2. Extrair ou criar helper que calcule progressão global do fundamental retornando também se o ano atual é último da sequência.
3. Consultar os `anos_academicos` da academia atual antes de decidir finalização e vínculos automáticos; a falta de oferta não deve impedir a atualização do estudante para o próximo ano global.
4. Ajustar o payload/evento de avaliação final para representar o cenário “aprovado, promovido para o próximo ano global, mas academia sem oferta desse ano fundamental”, se necessário.
5. Ajustar a projeção/processamento para não inferir finalização quando `aprovado=true`, ainda existe próximo ano global e a única pendência é a falta de oferta desse ano pela academia.
6. Garantir que estudante permaneça na mesma academia, com `status_escolar_fundamental = "em_andamento"` e `ano_escolar_fundamental` atualizado para o próximo ano global no cenário sem oferta do próximo ano.
7. Garantir que nenhuma turma ou vínculo automático seja criado quando não há próximo ano ofertado.
8. Atualizar documentação da API se houver novo campo de resposta/metadado.
9. Adicionar testes unitários e/ou de handler/projeção cobrindo todos os cenários de progressão fundamental.

## Cenários de teste mínimos

### Aprovação com próximo ano ofertado

- Estudante no `5_ano_fundamental`, academia oferece `6_ano_fundamental`, aprovado: deve avançar para `6_ano_fundamental` conforme comportamento atual e manter `status_escolar_fundamental = "em_andamento"`.

### Aprovação sem próximo ano ofertado

- Estudante no `5_ano_fundamental`, academia não oferece `6_ano_fundamental`, aprovado: deve continuar `status_escolar_fundamental = "em_andamento"`, passar para `ano_escolar_fundamental = "6_ano_fundamental"`, permanecer na mesma academia, não receber turma automática e não ser finalizado.
- Estudante no `8_ano_fundamental`, academia não oferece `9_ano_fundamental`, aprovado: deve continuar `em_andamento`, passar para `ano_escolar_fundamental = "9_ano_fundamental"`, sem finalização indevida e sem vínculo automático.
- Academia mista sem o próximo ano fundamental, mas com anos médios configurados em cursos: não deve usar ano médio como substituto do próximo fundamental.

### Conclusão real do fundamental

- Estudante no `9_ano_fundamental`, aprovado: deve finalizar o fundamental conforme regra atual, desde que `9_ano_fundamental` seja o último ano global canônico.

### Reprovação

- Estudante reprovado em qualquer ano: deve manter comportamento atual, sem promoção e sem finalização causada por esta regra.

### Rebuild e idempotência

- Rebuild de projeções a partir dos eventos deve manter o mesmo resultado: aprovado sem próximo ano ofertado continua `em_andamento`, com `ano_escolar_fundamental` no próximo ano global, não `finalizado`.
- Reprocessar o mesmo evento não deve criar vínculos duplicados nem alterar o estudante para `finalizado`.

## Critérios de aceite

- Aprovação do fundamental não finaliza o estudante quando ainda existe próximo ano global, mas a academia atual não oferta esse ano.
- Estudante permanece com `status_escolar_fundamental = "em_andamento"`, vinculado à mesma academia e com `ano_escolar_fundamental` atualizado para o próximo ano global no cenário sem próximo ano ofertado.
- Nenhuma turma, matéria, curso, período ou vínculo automático inválido é criado quando o próximo ano não existe na academia.
- Conclusão real do `9_ano_fundamental` continua finalizando o fundamental.
- Ensino médio e superior não têm comportamento alterado por esta tarefa.
- Eventos/projeções/rebuilds diferenciam ausência de próximo ano por fim real do ciclo versus ausência de oferta da academia.
- Testes cobrem aprovação com próximo ano ofertado, aprovação sem próximo ano ofertado, conclusão real do fundamental e reprovação.

## Observações importantes

- Esta tarefa deve ser implementada como regra de negócio de progressão, não como validação superficial de payload.
- Não resolver este caso transferindo automaticamente o estudante para outra academia.
- Não criar registros artificiais de turma ou ano acadêmico apenas para permitir a promoção.
- Se existir tarefa de gestão de anos acadêmicos da academia, esta regra deve ser compatível com ela: quando o ano seguinte for habilitado futuramente, outro fluxo explícito poderá rematricular/promover o estudante pendente.
- Preferir nomes de motivo auditáveis e estáveis, por exemplo `academia_sem_oferta_do_proximo_ano_academico_fundamental`.
