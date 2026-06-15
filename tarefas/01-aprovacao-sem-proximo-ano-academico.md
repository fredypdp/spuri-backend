# Regra de negócio: estudante aprovado sem ano acadêmico seguinte na academia

## Contexto atual

A avaliação final é o mecanismo que avança ou retém o estudante no nível acadêmico. Hoje o backend calcula `proximo_ano_academico` automaticamente e rejeita `proximo_ano_academico` enviado pelo cliente. Para fundamental a sequência é fixa de `1_ano_fundamental` a `9_ano_fundamental`; para médio/superior a sequência vem do curso do estudante.

Também existe uma regra documentada para escola: aprovado deve terminar com turma de destino válida quando houver próximo nível. Esse ponto precisa ser tratado de forma explícita quando a academia não oferece o próximo ano acadêmico ou não tem curso/turma configurados para ele.

## Problema

Quando `aprovado=true` e não existe próximo ano acadêmico configurado/oferecido pela academia, o sistema pode confundir dois cenários diferentes:

1. **Conclusão legítima do ciclo**: o estudante está no último ano/semestre do curso/ciclo e deve ser marcado como `finalizado`.
2. **Configuração incompleta da academia**: o estudante ainda não concluiu o ciclo, mas a academia não cadastrou/oferece o ano seguinte, curso seguinte, matéria/turma de destino ou semestre seguinte.

Sem uma regra clara, a IA poderia implementar uma progressão silenciosa para `finalizado`, aprovar sem turma, ou bloquear casos de conclusão legítima.

## Regra proposta

### 1. Separar “fim do ciclo” de “ano seguinte inexistente”

Ao registrar avaliação final:

- Se o estudante está no último nível real do ciclo/curso, `proximo_ano_academico = null` e a aprovação finaliza o ciclo.
- Se o estudante não está no último nível esperado, mas a academia não tem o próximo nível disponível, a aprovação deve ser bloqueada com erro de negócio.

### 2. Fundamental

- Sequência oficial: `1_ano_fundamental` … `9_ano_fundamental`.
- Se atual for `9_ano_fundamental` e aprovado, finaliza fundamental.
- Se atual for `1_ano_fundamental` … `8_ano_fundamental`, o próximo ano existe conceitualmente.
- Antes de aprovar, validar se a academia oferece o próximo ano em `academia.anos_academicos` quando a escola for fundamental/mista.
- Se a escola não oferecer o próximo ano, retornar erro: “academia não possui o ano acadêmico seguinte configurado/oferecido; regularize a configuração antes de aprovar”.

### 3. Médio

- A sequência deve vir de `curso.anos_academicos` do curso médio do estudante.
- Se o ano atual é o último elemento da sequência do curso, finaliza médio.
- Se o ano atual não está na sequência, bloquear por inconsistência.
- Se há próximo elemento na sequência, ele é obrigatório.
- Se o curso do estudante está ausente, inativo, deletado ou sem `anos_academicos`, bloquear aprovação.

### 4. Superior

- A sequência deve vir de `curso.periodos` ou `curso.anos_academicos`, conforme o modelo já usado pelo backend para cálculo do próximo nível.
- Se o período atual é o último, finaliza superior.
- Se não é o último e o próximo período não existe/está inativo, bloquear.

### 5. Turma de destino

Quando houver `proximo_ano_academico`:

- Para escola/fundamental/médio, exigir turma ativa compatível para destino.
- A turma destino deve pertencer à mesma academia, estar ativa, ter `nivel = proximo_ano_academico`, respeitar `curso_id` quando aplicável e ter capacidade disponível quando a configuração de lotação máxima existir.
- Se não houver turma compatível, bloquear a avaliação final antes de gravar evento.

Quando `proximo_ano_academico = null` por conclusão de ciclo:

- Não exigir turma destino.
- Remover o estudante das turmas acadêmicas do ciclo apenas se a regra de negócio de finalização exigir isso; caso contrário manter histórico e marcar status finalizado.

## Fluxo operacional

1. Academia chama `POST /academia/avaliacao-final`.
2. Backend identifica academia, ano letivo ativo e estudante.
3. Backend infere tipo de ensino do estudante.
4. Backend valida se `nivel_ano_academico_atual` corresponde ao estado atual do estudante.
5. Backend calcula a sequência aplicável.
6. Backend classifica o resultado:
   - `CONCLUI_CICLO`: último nível da sequência.
   - `AVANCA`: existe próximo nível.
   - `BLOQUEADO_CONFIGURACAO`: deveria haver próximo nível, mas a academia/curso/turma não está configurado.
7. Se `BLOQUEADO_CONFIGURACAO`, responder 400/409 com instrução prática para corrigir academia/curso/turma.
8. Se `AVANCA`, validar notas e turma destino.
9. Se `CONCLUI_CICLO`, validar notas e marcar status do ciclo como finalizado.
10. Persistir evento `AvaliacaoFinalEscolar` ou `AvaliacaoFinalSuperior` somente depois de todas as validações.

## Validações técnicas esperadas

- Não aceitar `proximo_ano_academico` no payload.
- Não aceitar aprovação se `ano_letivo` da academia estiver vazio.
- Não aceitar avaliação duplicada no mesmo ano letivo.
- Validar que o próximo ano calculado existe na configuração oficial da academia/curso.
- Validar que a turma destino existe antes de salvar evento quando houver progressão escolar.
- Usar transação quando a avaliação final também alterar projeção/estado de turma.

## Impacto em Event Sourcing

- A regra deve ficar antes de `RaiseEvent` no handler/aggregate.
- O evento deve conter `proximo_ano_academico = null` apenas quando for conclusão real de ciclo ou reprovação.
- Rebuild deve produzir exatamente o mesmo estado: estudante avançado, retido ou finalizado.

## Testes recomendados

- Aprovar 8.º fundamental sem `9_ano_fundamental` na academia: deve bloquear.
- Aprovar 9.º fundamental: deve finalizar ciclo sem exigir próximo ano.
- Aprovar 2.º médio em curso com 3 anos: deve ir para 3.º médio.
- Aprovar 2.º médio em curso configurado só até 2.º: deve finalizar somente se o curso realmente define 2.º como último.
- Aprovar estudante com curso médio ausente: deve bloquear.
- Aprovar com próximo ano existente, mas sem turma ativa compatível: deve bloquear.
