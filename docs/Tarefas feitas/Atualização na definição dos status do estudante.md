---
modificado: 2026-06-21 19:07
criado: 2026-06-09 19:07
---
# Atualização na definição dos status do estudante (feito)
Atualmente os status do estudante podem ser definidos diretamente, acontece que não é tão seguro, o ideal é eles serem atualizados como consequência de algum evento (como efeito colateral de acontecimentos reais do domínio). Esse documento descreve as atualizações que devem ser feitas para isso. Aplique as atualizações no código e banco de dados se necessário seguindo as orientações desse arquivo. No final atualize as duas documentações deixando elas completamente claras quanto ao funcionamento do sistema.

Não deixe nenhum resquício do código legado, porque o sistema ainda não tá em produção entao não tem lógica deixar esse código lá.

# 1. `status` — status geral do estudante

## Situações/eventos existentes

| Acontecimento existente                         | Evento atual                | Mudança de status | Observação                                                                                                                    |
| ----------------------------------------------- | --------------------------- | ----------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| Estudante é cadastrado/vinculado a uma academia | `EstudanteCriadoComVinculo` | `status = ativo`  | A projeção insere o estudante com `status = 'ativo'`.  O aggregate também aplica `e.Status = "ativo"` ao processar a criação. |

## Situações/eventos novos

| Acontecimento proposto                                           | Novo Evento                       | Mudança sugerida    |
| ---------------------------------------------------------------- | --------------------------------- | ------------------- |
| Estudante saiu da academia, mas o histórico deve ser preservado  | `EstudanteDesvinculadoDaAcademia` | `ativo → arquivado` |
| Estudante voltou para a academia após arquivamento/desvinculação | `EstudanteReintegrado`            | `arquivado → ativo` |

### Observação importante

O campo `status` agora já não deve receber o valor: `finalizado`

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

| Acontecimento proposto                                         | Evento sugerido                                                                                                      | Mudança sugerida                    |
| -------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- | ----------------------------------- |
| Estudante inicia o fundamental naquela academia                | `MatriculaFundamentalEfetivada`                                                                                      | `inativo → em_andamento`            |
| Estudante retoma o fundamental depois de interrupção           | `FundamentalRetomado`                                                                                                | `inativo → em_andamento`            |
| Estudante conclui o último ano do fundamental                  | Já existe via `AvaliacaoFinalEscolar`, mas poderia ser explicitado como `FundamentalConcluido` derivado da avaliação | `em_andamento → finalizado`         |
| Estudante tem equivalência/dispensa do fundamental reconhecida | `EquivalenciaFundamentalReconhecida`                                                                                 | `inativo/em_andamento → finalizado` |
| Estudante tem equivalência/dispensa do fundamental reconhecida | `EquivalenciaFundamentalReconhecida`                                                                                 | `inativo/em_andamento → finalizado` |
| Estudante sai da etapa fundamental sem concluir                | `FundamentalInterrompido`                                                                                            | `em_andamento → inativo`            |

---

# 3. `status_escolar_medio`

Valores:
- `inativo`
- `em_andamento`
- `finalizado`

O DTO do estudante possui `status_escolar_medio: StatusEscolar`. {line_range_start=212 line_range_end=216 path=docs/Spuri - API.md git_url="https://github.com/fredypdp/spuri-backend/blob/main/docs/Spuri - API.md#L212-L216"}

## Situações/eventos existentes

| Acontecimento existente                                     | Evento atual                   | Mudança de status                                                                                                 | Deve continuar?                                                                                                                                                                                           |
| ----------------------------------------------------------- | ------------------------------ | ----------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Estudante é criado/vinculado                                | `EstudanteCriadoComVinculo`    | Por padrão, `status_escolar_medio = inativo`; pode vir outro valor válido no payload                              | Sim, mas idealmente derivado da matrícula/ciclo.                                                                                                                                                          |
| Academia altera diretamente o status médio                  | `StatusEscolarMedioAtualizado` | Qualquer um dos três valores                                                                                      | **Não deveria continuar** na nova regra. O handler chama `AtualizarStatusEscolarMedio`, que emite esse evento.                                                                                            |
| Evento legado altera status escolar geral                   | `StatusEscolarAtualizado`      | Atualiza fundamental e médio juntos                                                                               | **Não deveria continuar**; é alteração direta/legada.                                                                                                                                                     |
| Avaliação final escolar aprovada no último ano do médio     | `AvaliacaoFinalEscolar`        | `status_escolar_medio = finalizado` quando `tipoEnsino = medio`, `aprovado = true` e não há próximo ano acadêmico | Sim. É alteração consequente de avaliação final.                                                                                                                                                          |
| Avaliação final escolar reprovada                           | `AvaliacaoFinalEscolar`        | Não altera status                                                                                                 | Sim. Reprovação não altera ano/status. {line_range_start=2036 line_range_end=2038 path=docs/Spuri - API.md git_url="https://github.com/fredypdp/spuri-backend/blob/main/docs/Spuri - API.md#L2036-L2038"} |
| Avaliação final aprovada, mas ainda há próximo ano do médio | `AvaliacaoFinalEscolar`        | Não altera status; altera `ano_escolar_medio`                                                                     | Sim.                                                                                                                                                                                                      |

## Situações/eventos não existentes, mas recomendados

| Acontecimento proposto                                               | Evento sugerido                                                                                             | Mudança sugerida                                                                                                       |
| -------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Estudante concluiu fundamental e foi matriculado no médio            | `MatriculaMedioEfetivada`                                                                                   | `status_escolar_medio: inativo → em_andamento`; normalmente `status_escolar_fundamental` já deveria estar `finalizado` |
| Estudante entra diretamente no médio por equivalência do fundamental | `EquivalenciaFundamentalReconhecida` + `MatriculaMedioEfetivada`                                            | fundamental `finalizado`; médio `em_andamento`                                                                         |
| Estudante troca de curso médio durante o médio                       | Já existe `CursoAlterado`, mas hoje ele **não muda status**; apenas exige que o médio esteja `em_andamento` | Sem mudança de status; manter `em_andamento`                                                                           |
| Estudante conclui o último ano do médio                              | Já existe via `AvaliacaoFinalEscolar`, mas poderia gerar/ser tratado como `MedioConcluido`                  | `em_andamento → finalizado`                                                                                            |
| Estudante abandona/interrompe o médio                                | `MedioInterrompido`                                                                                         | `em_andamento → inativo`                                                                                               |
| Estudante retorna ao médio depois de interrupção                     | `MedioRetomado`                                                                                             | `inativo → em_andamento`                                                                                               |
| Estudante recebe equivalência/certificação externa do médio          | `EquivalenciaMedioReconhecida`                                                                              | `inativo/em_andamento → finalizado`                                                                                    |

### Observação sobre curso médio

Hoje `AlterarCurso` só permite alterar curso médio se `status_escolar_medio == "em_andamento"`.  Isso reforça que o evento de alteração de curso **não deveria ativar o médio por si só**; antes disso deveria existir um acontecimento como `MatriculaMedioEfetivada`. Mas o que eu gostaria é que para o estudante alterar o curso ele só precisa já ter o fundamental finalizado + estar numa academia do nível médio, isso porque talvez tenha interrompido/suspenso o médio e queira retomar mas em outro curso, então depender do status "em_andamento" não seria uma boa ideia"

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

| Acontecimento proposto                                                 | Evento sugerido                                                                                | Mudança sugerida                             |
| ---------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- | -------------------------------------------- |
| Estudante concluiu médio e foi matriculado no superior                 | `MatriculaSuperiorEfetivada`                                                                   | `status_superior: inativo → em_andamento`    |
| Estudante ingressou no superior por equivalência/transferência externa | `IngressoSuperiorPorEquivalenciaRegistrado`                                                    | `inativo → em_andamento`                     |
| Estudante alterou curso superior                                       | Já existe `CursoAlterado`, mas hoje exige `status_superior = em_andamento` e não muda status   | Sem mudança de status; manter `em_andamento` |
| Estudante conclui o último semestre/ano superior                       | Já existe via `AvaliacaoFinalSuperior`, mas poderia gerar/ser tratado como `SuperiorConcluido` | `em_andamento → finalizado`                  |
| Estudante tranca curso superior                                        | `SuperiorTrancado`                                                                             | `em_andamento → inativo`                     |
| Estudante reabre matrícula após trancamento                            | `MatriculaSuperiorReativada`                                                                   | `inativo → em_andamento`                     |
| Estudante abandona o superior                                          | `SuperiorAbandonado`                                                                           | `em_andamento → inativo`                     |

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

| Endpoint novo sugerido                                     | Evento gerado                   |
| ---------------------------------------------------------- | ------------------------------- |
| `POST /academia/estudante/:codigo/matricula/fundamental`   | `MatriculaFundamentalEfetivada` |
| `POST /academia/estudante/:codigo/matricula/medio`         | `MatriculaMedioEfetivada`       |
| `POST /academia/estudante/:codigo/matricula/superior`      | `MatriculaSuperiorEfetivada`    |
| `POST /academia/estudante/:codigo/interrupcao/fundamental` | `FundamentalInterrompido`       |
| `POST /academia/estudante/:codigo/interrupcao/medio`       | `MedioInterrompido`             |
| `POST /academia/estudante/:codigo/trancamento/superior`    | `SuperiorTrancado`              |
| `POST /academia/estudante/:codigo/arquivar`                | `EstudanteArquivado`            |
| `POST /academia/estudante/:codigo/reativar`                | `EstudanteReativado`            |
Para os novos eventos do status geral:
- EstudanteDesvinculadoDaAcademia: deve ser criada uma rota para a academia desvincular o estudante. Na estrutura de dados desse evento deve ter o código da academia e do estudante, o motivo no campo "motivo", e o nível que ele tava quando foi desvinculado (vai saber pelo status_escolar_fundamental/status_escolar_medio/status_superior, e definir usando respectivamente ano_escolar_fundamental/ano_escolar_medio ou o ano_superior + semestre_atual) - `POST /academia/estudante/:codigo/desvincular`
- EstudanteReintegrado: deve ser criada uma rota para a academia reintegrar o estudante. Vai seguir uma lógica parecida com a matrícula, reutilizando um pouco dos daods do estudante que já foram gravado na plataforma, sobrescrevendo assim os dados dele que estavam gravados antes. - `POST /academia/estudante/:codigo/revincular`
	- Para escola: a escola define o ano escolar do fundamental caso seja reingresso nesse nível, ou ano escolar do ensino médio + o curso do estudante a ser reintegrado
	- Para ensino superior: a instituição define o o curso do estudante a ser reintegrado, e o sistema define o `ano_superior` como `1_ano_superior` e `semestre_atual` como `1`

# Algumas explicações

Quando eu falei de **equivalência**, **dispensa**, **certificação externa**, **ingresso direto** e termos parecidos, eu estava falando de uma categoria de acontecimentos que servem para explicar por que um estudante pode ter uma etapa marcada como `finalizado` ou pode iniciar uma etapa mais avançada **sem ter passado pelo fluxo normal dentro do sistema**.

Hoje, o sistema trabalha com estes status escolares:

```
inativo | em_andamento | finalizado
```

Isso aparece na documentação como `StatusEscolar`.

O fluxo normal de avanço/conclusão que já existe é a **avaliação final**: se o estudante for aprovado, o backend calcula o próximo ano; se chegou ao fim da sequência, o ciclo pode ser considerado concluído. Mas os casos de **equivalência/dispensa/certificação externa** são situações em que a academia diz:
> “Esse estudante já cumpriu essa etapa, ou está autorizado a não cursá-la aqui, por um motivo reconhecido.”

Ou seja: o sistema não deveria simplesmente “setar status”. Ele deveria registrar **qual foi o acontecimento**.

---

## 1. Equivalência

### O que é?

**Equivalência** é quando a academia reconhece que algo feito fora do sistema tem o mesmo valor acadêmico de uma etapa interna.

Em termos simples:

> O estudante não fez aquele percurso dentro desta plataforma, mas trouxe uma prova/documento/situação que a academia aceita como equivalente.

### Exemplo prático

Um estudante chega para se matricular no ensino médio e apresenta histórico escolar de outra escola mostrando que concluiu o fundamental.

Nesse caso, a academia não deveria apenas fazer:

```
status_escolar_fundamental = finalizado
```

Ela deveria registrar algo como:

```
EquivalenciaFundamentalReconhecida
```

Esse evento explica o motivo do fundamental estar finalizado.

---

## 2. Certificação externa

### O que é?

**Certificação externa** é quando o estudante apresenta um certificado formal emitido fora da academia/plataforma, e a academia reconhece esse certificado.

É mais forte/específico do que “equivalência”, porque normalmente envolve um documento de conclusão.

### Exemplo prático

O estudante apresenta um certificado externo de conclusão do ensino médio.

### Diferença entre equivalência e certificação externa

|Termo|Exemplo|
|---|---|
|Equivalência|Histórico de outra escola mostra que o estudante cursou algo equivalente|
|Certificação externa|Certificado formal comprova conclusão da etapa|

## 3. Conclusão externa reconhecida

### O que é?

Esse é, na minha opinião, o nome mais claro para muitos desses casos.

**Conclusão externa reconhecida** significa:

> O estudante concluiu essa etapa fora da plataforma, e a academia reconheceu essa conclusão.

### Exemplo

```
ConclusaoFundamentalExternaReconhecidaConclusaoMedioExternaReconhecida
```

Esse nome é mais fácil de entender do que “equivalência” ou “certificação”.