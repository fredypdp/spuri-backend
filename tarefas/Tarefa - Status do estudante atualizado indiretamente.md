---
modificado: 2026-06-10 22:10
criado: 2026-05-19 18:06
---
## Entendimento da atualização desejada

A regra nova seria:

> **Os campos de status do estudante não devem mais ser alterados por comandos/eventos cujo único objetivo é “setar status”.**  
> Eles devem mudar como **efeito colateral de acontecimentos reais do domínio**: matrícula, avaliação final, conclusão de ciclo, interrupção, reingresso, arquivamento etc.

Hoje o sistema ainda possui eventos e endpoints de alteração direta de status escolar, como `StatusEscolarFundamentalAtualizado`, `StatusEscolarMedioAtualizado` e `StatusSuperiorAtualizado`. A projeção escuta esses eventos e atualiza diretamente os campos de status correspondentes.  Esses eventos são acionados por endpoints diretos como `PUT /academia/estudante/:codigo/status-escolar-fundamental`, `PUT /academia/estudante/:codigo/status-escolar-medio` e `PUT /academia/estudante/:codigo/status-superior`. 

Abaixo listo, para cada status, **os acontecimentos já existentes** que hoje alteram ou poderiam ser usados como fonte da alteração, e **os acontecimentos ainda não existentes** que fariam sentido criar para atender à nova regra.

---

# 1. `status` — status geral do estudante

Valores observados:

- Na documentação do DTO: `ativo | inativo`. {line_range_start=212 line_range_end=214 path=docs/Spuri - API.md git_url="https://github.com/fredypdp/spuri-backend/blob/main/docs/Spuri - API.md#L212-L214"}
    
- No schema inicial do banco: `inativo`, `ativo`, `finalizado`. 
    
- O middleware de autenticação só deixa passar estudante com `status == "ativo"`. 
    

## Situações/eventos existentes

|Acontecimento existente|Evento atual|Mudança de status|Observação|
|---|---|---|---|
|Estudante é cadastrado/vinculado a uma academia|`EstudanteCriadoComVinculo`|`status = ativo`|A projeção insere o estudante com `status = 'ativo'`.  O aggregate também aplica `e.Status = "ativo"` ao processar a criação.|

## Situações/eventos não existentes, mas recomendados

|Acontecimento proposto|Evento sugerido|Mudança sugerida|
|---|---|---|
|Estudante saiu da academia, mas o histórico deve ser preservado|`EstudanteArquivado` ou `EstudanteDesvinculadoDaAcademia`|`ativo → inativo` ou idealmente `ativo → arquivado`|
|Estudante voltou para a academia após arquivamento/desvinculação|`EstudanteReativado` ou `EstudanteReintegrado`|`inativo/arquivado → ativo`|
|Estudante concluiu toda a trajetória vinculada à academia|`TrajetoriaAcademicaConcluida`|`ativo → finalizado`|
|Estudante foi bloqueado administrativamente por motivo disciplinar/administrativo|`EstudanteBloqueado` ou `EstudanteSuspenso`|`ativo → inativo`|
|Bloqueio/suspensão foi removido|`BloqueioEstudanteRemovido` ou `EstudanteReabilitado`|`inativo → ativo`|

### Observação importante

A própria documentação já aponta que hoje **não existe mecanismo de arquivar estudante**, e recomenda adicionar um status `arquivado` para estudantes que saíram da academia mantendo histórico. {line_range_start=1038 line_range_end=1042 path=docs/Spuri - Documentação.md git_url="https://github.com/fredypdp/spuri-backend/blob/main/docs/Spuri - Documentação.md#L1038-L1042"}

---

# 2. `status_escolar_fundamental`

Valores:

- `inativo`
    
- `em_andamento`
    
- `finalizado`
    

Esse tipo é definido como `StatusEscolar = 'inativo' | 'em_andamento' | 'finalizado'`. {line_range_start=108 line_range_end=114 path=docs/Spuri - API.md git_url="https://github.com/fredypdp/spuri-backend/blob/main/docs/Spuri - API.md#L108-L114"}

## Situações/eventos existentes

| Acontecimento existente                                       | Evento atual                         | Mudança de status                                                                                                             | Deve continuar?                                                                                                                                                                                                                                                 |
| ------------------------------------------------------------- | ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Estudante é criado/vinculado                                  | `EstudanteCriadoComVinculo`          | Por padrão, `status_escolar_fundamental = em_andamento`; pode vir outro valor válido no payload                               | Sim, mas o ideal é a criação derivar isso de dados de matrícula/ciclo, não receber status arbitrário. O aggregate define defaults `fundamental = em_andamento`, `medio = inativo`, `superior = inativo`.                                                        |
| Academia altera diretamente o status fundamental              | `StatusEscolarFundamentalAtualizado` | Qualquer um dos três valores                                                                                                  | **Não deveria continuar** na nova regra, porque é alteração direta. O handler chama `AtualizarStatusEscolarFundamental`, que emite esse evento.                                                                                                                 |
| Evento legado altera status escolar geral                     | `StatusEscolarAtualizado`            | Atualiza fundamental e médio juntos                                                                                           | **Não deveria continuar**; é alteração direta/legada. A projeção ainda trata esse evento e seta `status_escolar_fundamental` e `status_escolar_medio` com o mesmo valor.                                                                                        |
| Avaliação final escolar aprovada no último ano do fundamental | `AvaliacaoFinalEscolar`              | `status_escolar_fundamental = finalizado` quando `tipoEnsino = fundamental`, `aprovado = true` e não há próximo ano acadêmico | Sim. Este é o melhor exemplo já existente de status mudando como consequência de um acontecimento real.                                                                                                                                                         |
| Avaliação final escolar reprovada                             | `AvaliacaoFinalEscolar`              | Não altera status                                                                                                             | Sim. A regra atual registra a avaliação, mas reprovação não altera ano/status. {line_range_start=845 line_range_end=850 path=docs/Spuri - Documentação.md git_url="https://github.com/fredypdp/spuri-backend/blob/main/docs/Spuri - Documentação.md#L845-L850"} |
| Avaliação final aprovada, mas ainda há próximo ano            | `AvaliacaoFinalEscolar`              | Não altera status; altera apenas o ano acadêmico                                                                              | Sim. A projeção altera `ano_escolar_fundamental` quando existe `ProximoAnoAcademico`.                                                                                                                                                                           |

## Situações/eventos não existentes, mas recomendados

| Acontecimento proposto                                            | Evento sugerido                                                                                                      | Mudança sugerida                                                                                                              |
| ----------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| Estudante inicia o fundamental naquela academia                   | `MatriculaFundamentalEfetivada`                                                                                      | `inativo → em_andamento`                                                                                                      |
| Estudante retoma o fundamental depois de interrupção              | `FundamentalRetomado`                                                                                                | `inativo → em_andamento`                                                                                                      |
| Estudante conclui o último ano do fundamental                     | Já existe via `AvaliacaoFinalEscolar`, mas poderia ser explicitado como `FundamentalConcluido` derivado da avaliação | `em_andamento → finalizado`                                                                                                   |
| Estudante tem equivalência do fundamental reconhecida             | `EquivalenciaFundamentalReconhecida`                                                                                 | `inativo/em_andamento → finalizado`                                                                                           |
| Estudante sai da etapa fundamental sem concluir                   | `FundamentalInterrompido`                                                                                            | `em_andamento → inativo`                                                                                                      |
| Estudante é transferido para outra academia durante o fundamental | `TransferenciaFundamentalRegistrada`                                                                                 | `em_andamento → inativo` ou manter status escolar e alterar `status` geral para `arquivado`, dependendo da decisão de produto |

---

# 3. `status_escolar_medio`

Valores:

- `inativo`
    
- `em_andamento`
    
- `finalizado`
    

O DTO do estudante possui `status_escolar_medio: StatusEscolar`. {line_range_start=212 line_range_end=216 path=docs/Spuri - API.md git_url="https://github.com/fredypdp/spuri-backend/blob/main/docs/Spuri - API.md#L212-L216"}

## Situações/eventos existentes

|Acontecimento existente|Evento atual|Mudança de status|Deve continuar?|
|---|---|---|---|
|Estudante é criado/vinculado|`EstudanteCriadoComVinculo`|Por padrão, `status_escolar_medio = inativo`; pode vir outro valor válido no payload|Sim, mas idealmente derivado da matrícula/ciclo.|
|Academia altera diretamente o status médio|`StatusEscolarMedioAtualizado`|Qualquer um dos três valores|**Não deveria continuar** na nova regra. O handler chama `AtualizarStatusEscolarMedio`, que emite esse evento.|
|Evento legado altera status escolar geral|`StatusEscolarAtualizado`|Atualiza fundamental e médio juntos|**Não deveria continuar**; é alteração direta/legada.|
|Avaliação final escolar aprovada no último ano do médio|`AvaliacaoFinalEscolar`|`status_escolar_medio = finalizado` quando `tipoEnsino = medio`, `aprovado = true` e não há próximo ano acadêmico|Sim. É alteração consequente de avaliação final.|
|Avaliação final escolar reprovada|`AvaliacaoFinalEscolar`|Não altera status|Sim. Reprovação não altera ano/status. {line_range_start=2036 line_range_end=2038 path=docs/Spuri - API.md git_url="https://github.com/fredypdp/spuri-backend/blob/main/docs/Spuri - API.md#L2036-L2038"}|
|Avaliação final aprovada, mas ainda há próximo ano do médio|`AvaliacaoFinalEscolar`|Não altera status; altera `ano_escolar_medio`|Sim.|

## Situações/eventos não existentes, mas recomendados

|Acontecimento proposto|Evento sugerido|Mudança sugerida|
|---|---|---|
|Estudante concluiu fundamental e foi matriculado no médio|`MatriculaMedioEfetivada`|`status_escolar_medio: inativo → em_andamento`; normalmente `status_escolar_fundamental` já deveria estar `finalizado`|
|Estudante entra diretamente no médio por equivalência do fundamental|`EquivalenciaFundamentalReconhecida` + `MatriculaMedioEfetivada`|fundamental `finalizado`; médio `em_andamento`|
|Estudante troca de curso médio durante o médio|Já existe `CursoAlterado`, mas hoje ele **não muda status**; apenas exige que o médio esteja `em_andamento`|Sem mudança de status; manter `em_andamento`|
|Estudante conclui o último ano do médio|Já existe via `AvaliacaoFinalEscolar`, mas poderia gerar/ser tratado como `MedioConcluido`|`em_andamento → finalizado`|
|Estudante abandona/interrompe o médio|`MedioInterrompido`|`em_andamento → inativo`|
|Estudante retorna ao médio depois de interrupção|`MedioRetomado`|`inativo → em_andamento`|
|Estudante recebe equivalência/certificação externa do médio|`EquivalenciaMedioReconhecida`|`inativo/em_andamento → finalizado`|

### Observação sobre curso médio

Hoje `AlterarCurso` só permite alterar curso médio se `status_escolar_medio == "em_andamento"`.  Isso reforça que o evento de alteração de curso **não deveria ativar o médio por si só**; antes disso deveria existir um acontecimento como `MatriculaMedioEfetivada`.

---

# 4. `status_superior`

Valores:

- `inativo`
    
- `em_andamento`
    
- `finalizado`
    

O DTO do estudante possui `status_superior: StatusEscolar`. {line_range_start=212 line_range_end=216 path=docs/Spuri - API.md git_url="https://github.com/fredypdp/spuri-backend/blob/main/docs/Spuri - API.md#L212-L216"}

## Situações/eventos existentes

|Acontecimento existente|Evento atual|Mudança de status|Deve continuar?|
|---|---|---|---|
|Estudante é criado/vinculado|`EstudanteCriadoComVinculo`|Por padrão, `status_superior = inativo`; pode vir outro valor válido no payload|Sim, mas idealmente derivado de matrícula superior.|
|Academia altera diretamente o status superior|`StatusSuperiorAtualizado`|Qualquer um dos três valores|**Não deveria continuar** na nova regra. O handler chama `AtualizarStatusSuperior`, que emite esse evento.|
|Avaliação final superior aprovada no último semestre/ano|`AvaliacaoFinalSuperior`|`status_superior = finalizado` quando `tipoEnsino = superior`, `aprovado = true` e não há próximo ano acadêmico|Sim. É consequência de avaliação final.|
|Avaliação final superior reprovada|`AvaliacaoFinalSuperior`|Não altera status|Sim. Reprovação não altera avanço/status.|
|Avaliação final superior aprovada, mas ainda há próximo semestre/ano|`AvaliacaoFinalSuperior`|Não altera status; altera `ano_superior`/avanço acadêmico|Sim.|

## Situações/eventos não existentes, mas recomendados

|Acontecimento proposto|Evento sugerido|Mudança sugerida|
|---|---|---|
|Estudante concluiu médio e foi matriculado no superior|`MatriculaSuperiorEfetivada`|`status_superior: inativo → em_andamento`|
|Estudante ingressou no superior por equivalência/transferência externa|`IngressoSuperiorPorEquivalenciaRegistrado`|`inativo → em_andamento`|
|Estudante alterou curso superior|Já existe `CursoAlterado`, mas hoje exige `status_superior = em_andamento` e não muda status|Sem mudança de status; manter `em_andamento`|
|Estudante conclui o último semestre/ano superior|Já existe via `AvaliacaoFinalSuperior`, mas poderia gerar/ser tratado como `SuperiorConcluido`|`em_andamento → finalizado`|
|Estudante tranca curso superior|`SuperiorTrancado`|`em_andamento → inativo`|
|Estudante reabre matrícula após trancamento|`SuperiorReaberto` ou `MatriculaSuperiorReativada`|`inativo → em_andamento`|
|Estudante abandona o superior|`SuperiorAbandonado`|`em_andamento → inativo`|
|Estudante é transferido para outra instituição no superior|`TransferenciaSuperiorRegistrada`|`em_andamento → inativo` ou status geral `arquivado`|

### Regra já existente para avanço do superior

O método direto atual `AtualizarStatusSuperior` já possui uma regra: o superior só pode avançar para `em_andamento` ou `finalizado` se fundamental e médio estiverem `finalizado` ou `inativo`.  Na nova modelagem, essa regra deveria sair do “set status superior” e ir para o acontecimento real, por exemplo `MatriculaSuperiorEfetivada`.

---

# Eventos diretos que deveriam ser removidos/depreciados

Para cumprir a nova regra, estes eventos não deveriam mais ser usados como eventos de domínio públicos:

|Evento atual|Problema|
|---|---|
|`StatusEscolarFundamentalAtualizado`|Representa uma alteração direta de status, não um acontecimento real.|
|`StatusEscolarMedioAtualizado`|Mesmo problema.|
|`StatusSuperiorAtualizado`|Mesmo problema.|
|`StatusEscolarAtualizado`|Legado; altera fundamental e médio diretamente com o mesmo valor.|

Esses eventos aparecem na projeção de estudante e hoje atualizam diretamente os campos de status.  A documentação também lista esses eventos como “status alterado”, o que precisaria ser atualizado para refletir os novos eventos de negócio. {line_range_start=336 line_range_end=345 path=docs/Spuri - Documentação.md git_url="https://github.com/fredypdp/spuri-backend/blob/main/docs/Spuri - Documentação.md#L336-L345"}

---

# Endpoints diretos que deveriam ser removidos/depreciados

Hoje existem endpoints síncronos para alteração direta:

- `PUT /academia/estudante/:codigo/status-escolar-fundamental`
    
- `PUT /academia/estudante/:codigo/status-escolar-medio`
    
- `PUT /academia/estudante/:codigo/status-superior`
    

Eles estão registrados nas rotas. 

Também existem fluxos batch/async que reaproveitam os mesmos handlers diretos:

- Batch síncrono `AtualizarStatusEscolarBatch`, que recebe `tipo` e `novo_status`. 
    
- Job item async `AtualizarStatusEscolarJobItem`, que também recebe `tipo` e `novo_status`. 
    
- Rota async `PUT /academia/estudante/status-escolar/async`. 
    

Na nova regra, esses endpoints deveriam ser substituídos por endpoints de acontecimentos, por exemplo:

|Endpoint novo sugerido|Evento gerado|
|---|---|
|`POST /academia/estudante/:codigo/matricula/fundamental`|`MatriculaFundamentalEfetivada`|
|`POST /academia/estudante/:codigo/matricula/medio`|`MatriculaMedioEfetivada`|
|`POST /academia/estudante/:codigo/matricula/superior`|`MatriculaSuperiorEfetivada`|
|`POST /academia/estudante/:codigo/interrupcao/fundamental`|`FundamentalInterrompido`|
|`POST /academia/estudante/:codigo/interrupcao/medio`|`MedioInterrompido`|
|`POST /academia/estudante/:codigo/trancamento/superior`|`SuperiorTrancado`|
|`POST /academia/estudante/:codigo/arquivar`|`EstudanteArquivado`|
|`POST /academia/estudante/:codigo/reativar`|`EstudanteReativado`|

---

# Resumo por status

|Status|Eventos existentes que mudam status de forma aceitável|Eventos existentes que mudam status diretamente e deveriam sair|Eventos novos recomendados|
|---|---|---|---|
|`status`|`EstudanteCriadoComVinculo → ativo`|Nenhum endpoint direto encontrado para estudante, mas schema permite `finalizado` sem evento claro|`EstudanteArquivado`, `EstudanteReativado`, `TrajetoriaAcademicaConcluida`, `EstudanteSuspenso`|
|`status_escolar_fundamental`|`EstudanteCriadoComVinculo`, `AvaliacaoFinalEscolar` no último ano aprovado|`StatusEscolarFundamentalAtualizado`, `StatusEscolarAtualizado`|`MatriculaFundamentalEfetivada`, `FundamentalInterrompido`, `FundamentalRetomado`, `EquivalenciaFundamentalReconhecida`, `FundamentalConcluido`|
|`status_escolar_medio`|`EstudanteCriadoComVinculo`, `AvaliacaoFinalEscolar` no último ano aprovado|`StatusEscolarMedioAtualizado`, `StatusEscolarAtualizado`|`MatriculaMedioEfetivada`, `MedioInterrompido`, `MedioRetomado`, `EquivalenciaMedioReconhecida`, `MedioConcluido`|
|`status_superior`|`EstudanteCriadoComVinculo`, `AvaliacaoFinalSuperior` no último semestre/ano aprovado|`StatusSuperiorAtualizado`|`MatriculaSuperiorEfetivada`, `SuperiorTrancado`, `SuperiorReaberto`, `SuperiorAbandonado`, `TransferenciaSuperiorRegistrada`, `SuperiorConcluido`|

---

# Recomendações de implementação

1. **Manter `AvaliacaoFinalEscolar` e `AvaliacaoFinalSuperior` como fonte de conclusão**, porque já são acontecimentos reais e já finalizam o status quando não há próximo ano acadêmico. 
    
2. **Depreciar/remover os eventos `Status...Atualizado`**, porque eles representam alteração direta de status. 
    
3. **Substituir endpoints diretos de status por endpoints de acontecimento**, especialmente matrícula, interrupção, reingresso, arquivamento e conclusão.
    
4. **Criar eventos específicos para entrada/início de ciclo**, porque hoje o sistema consegue finalizar ciclos pela avaliação final, mas não tem um evento semântico claro para “começou fundamental/médio/superior”.
    
5. **Decidir se `status = finalizado` geral ainda faz sentido**. O banco permite esse valor, mas a documentação do DTO cita apenas `ativo | inativo`. {line_range_start=212 line_range_end=214 path=docs/Spuri - API.md git_url="https://github.com/fredypdp/spuri-backend/blob/main/docs/Spuri - API.md#L212-L214"} Se a trajetória concluída for relevante, vale criar `TrajetoriaAcademicaConcluida`; se não, remover `finalizado` do status geral e manter finalizações apenas nos status por ciclo.