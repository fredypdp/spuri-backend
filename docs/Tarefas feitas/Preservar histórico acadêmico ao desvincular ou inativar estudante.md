---
criado: 2026-06-28 00:00
origem: solicitação direta do usuário
status: pronto_para_implementacao
modificado: 2026-06-28 19:07
---

# Preservar histórico acadêmico ao desvincular ou inativar estudante (feito)

## Prompt recomendado para executar a atualização

Implemente no backend a garantia de que nenhum evento que desvincule o estudante de uma academia, interrompa o vínculo, tranque matrícula, inactive temporariamente ou altere o status para uma condição sem vínculo ativo apague, reinicie ou substitua o histórico acadêmico já consolidado do estudante. Quando o estudante reingressar ou for revinculado à mesma academia ou a outra academia, ele deve continuar no estado acadêmico anterior se o curso/nível acadêmico não tiver mudado. A única exceção esperada é mudança efetiva de curso: no ensino superior, se o estudante mudar de curso, ele deve iniciar no `1_semestre` do novo curso, com compatibilidade em `1_ano_superior`; no ensino médio, se mudar de curso, ele deve iniciar no `1_ano_medio` do novo curso. Em todos os casos, o histórico anterior deve continuar consultável e não deve ser perdido.

## Contexto do problema

Hoje há risco de eventos de inativação, interrupção, trancamento, desvínculo ou revínculo recalcularem o estado acadêmico do estudante como se fosse um cadastro novo. Isso pode fazer o estudante voltar ao início do percurso acadêmico mesmo quando ele apenas ficou temporariamente inativo ou foi desvinculado e depois retornou sem mudança real de curso.

A regra de negócio desejada é separar claramente:

1. **Histórico acadêmico**: registros de progresso, notas, faltas, aprovações/reprovações, turmas, avaliações, anos/semestres cursados, eventos e vínculos anteriores. Deve ser imutável ou preservado por eventos compensatórios, nunca apagado ou resetado por inativação/desvínculo.
2. **Vínculo atual**: estado operacional que indica se o estudante está ativo, inativo, interrompido, trancado, desvinculado, transferido, revinculado ou reingressado.
3. **Posição acadêmica atual**: ano/semestre em que o estudante deve continuar quando houver retorno sem mudança de curso, ou posição inicial do novo curso quando houver mudança real de curso.

Exemplo esperado:

```json
{
  "estudante_id": "uuid-do-estudante",
  "academia_id": "uuid-da-academia",
  "curso_id": "uuid-do-curso",
  "status": "trancado",
  "semestre_atual": "4_semestre",
  "ano_superior": "2_ano_superior"
}
```

Se o estudante reingressar no mesmo curso depois do trancamento:

```json
{
  "estudante_id": "uuid-do-estudante",
  "academia_id": "uuid-da-academia",
  "curso_id": "uuid-do-curso",
  "status": "ativo",
  "semestre_atual": "4_semestre",
  "ano_superior": "2_ano_superior"
}
```

Se o estudante reingressar mudando para outro curso superior:

```json
{
  "estudante_id": "uuid-do-estudante",
  "academia_id": "uuid-da-academia",
  "curso_id": "uuid-do-novo-curso",
  "status": "ativo",
  "semestre_atual": "1_semestre",
  "ano_superior": "1_ano_superior"
}
```

Para ensino médio, se o estudante mudar de curso médio, deve retornar ao início do novo curso:

```json
{
  "estudante_id": "uuid-do-estudante",
  "academia_id": "uuid-da-academia",
  "curso_id": "uuid-do-novo-curso-medio",
  "status": "ativo",
  "ano_escolar_medio": "1_ano_medio"
}
```

## Estado atual a verificar no código

Antes de implementar, mapear todos os fluxos que podem alterar vínculo, status ou progressão do estudante. A implementação deve verificar especialmente:

- eventos do aggregate de estudante relacionados a criação, matrícula, vínculo, desvínculo, inativação, interrupção, trancamento, reingresso, transferência e alteração de curso;
- projeção de estudantes e qualquer lógica que inicialize `ano_escolar_fundamental`, `ano_escolar_medio`, `ano_superior` ou `semestre_atual`;
- handlers de estudante, matrícula, solicitações de matrícula e fluxos administrativos que possam recriar vínculo;
- migrações e projeções que tenham defaults de ano/semestre inicial;
- jobs ou reconstrução de projeções que possam derivar estado atual ignorando eventos históricos.

A regra nova deve evitar inicializações automáticas para `1_ano_medio`, `1_ano_superior` ou `1_semestre` quando o estudante já tem estado acadêmico anterior compatível com o curso/nível para o qual está retornando.

## Decisão de contrato

1. Eventos de **inativação, interrupção, trancamento ou desvínculo** não podem zerar campos de progressão acadêmica nem apagar histórico.
2. Eventos de **revínculo/reingresso sem mudança de curso** devem restaurar ou manter a posição acadêmica anterior do estudante.
3. Eventos de **revínculo/reingresso com mudança de curso no superior** devem posicionar o estudante no `1_semestre` do novo curso e derivar `1_ano_superior`.
4. Eventos de **revínculo/reingresso com mudança de curso no médio** devem posicionar o estudante no `1_ano_medio` do novo curso.
5. Mudanças de academia sem mudança de curso/nível equivalente não devem, por si só, resetar a progressão. Se houver regra de equivalência entre cursos/academias, ela deve ser explícita e auditável.
6. O histórico anterior deve permanecer consultável mesmo quando o estudante inicia um novo curso do começo.
7. A decisão de resetar progressão só pode ocorrer por evento de mudança de curso ou comando explícito de reinício acadêmico, nunca como efeito colateral de status/vínculo.

## Escopo por tipo de ensino

### Ensino fundamental

- Preservar `ano_escolar_fundamental` e histórico quando houver inativação/desvínculo e retorno sem mudança de trajetória acadêmica.
- Não reiniciar para `1_ano_fundamental` automaticamente apenas porque o estudante ficou inativo, transferido ou desvinculado.
- Se houver regra futura de mudança de escola ou equivalência de anos, ela deve ser tratada separadamente e sem apagar histórico.

### Ensino médio

- Preservar `ano_escolar_medio` quando o estudante retornar ao mesmo curso médio ou a vínculo equivalente sem mudança real de curso.
- Se houver mudança de curso médio, iniciar o vínculo atual em `1_ano_medio` do novo curso.
- Manter histórico do curso médio anterior, incluindo notas, faltas, avaliações, turmas e aprovações/reprovações.

### Ensino superior

- Preservar `semestre_atual` como fonte operacional da progressão e `ano_superior` como campo derivado/compatibilidade quando o estudante retornar ao mesmo curso superior.
- Se houver mudança de curso superior, iniciar o novo vínculo em `1_semestre` e derivar `1_ano_superior`.
- Não derivar `semestre_atual` apenas de status ativo/inativo. Status não é progressão.
- Não apagar histórico curricular, notas, faltas, avaliações, turmas, equivalências, mensalidades/ledger ou eventos do curso anterior.

## Objetivos funcionais

### 1. Identificar eventos que não podem resetar histórico

Criar ou ajustar validações para que os seguintes tipos de eventos não reinicializem progressão nem removam histórico:

- estudante inativado;
- matrícula trancada;
- vínculo interrompido;
- estudante desvinculado da academia;
- estudante transferido ou com transferência em processamento;
- estudante reativado;
- estudante revinculado;
- estudante reingressado sem mudança de curso.

Esses eventos podem alterar status, datas, motivo e vínculo atual, mas devem preservar campos acadêmicos e registros históricos.

### 2. Diferenciar retorno sem mudança de curso de mudança real de curso

Ao reingressar ou revincular, comparar o curso anterior com o novo curso:

- Se `curso_id` anterior e novo forem iguais, manter progressão anterior.
- Se o nível/curso for equivalente por regra explícita, manter progressão conforme regra de equivalência.
- Se houver mudança para outro curso superior, iniciar em `1_semestre` e `1_ano_superior`.
- Se houver mudança para outro curso médio, iniciar em `1_ano_medio`.
- Se o estudante não tiver histórico anterior no nível/curso, usar o comportamento de cadastro inicial já existente.

A comparação deve ser feita com identificadores canônicos (`curso_id`, `nivel`, `academia_id` quando necessário), não por nome textual do curso.

### 3. Preservar histórico em projeções e reconstruções

Garantir que reconstrução de projeções por replay de eventos produza o mesmo resultado esperado:

- eventos de status/vínculo não devem limpar progressão;
- eventos antigos de notas, faltas, turmas e avaliações continuam vinculados ao histórico;
- o vínculo atual pode apontar para novo curso, mas vínculos anteriores continuam disponíveis no histórico;
- defaults de cadastro inicial não devem sobrescrever valores anteriores durante replay.

### 4. Adicionar testes regressivos

Criar testes cobrindo pelo menos estes cenários:

1. estudante superior em `4_semestre` é trancado e depois reativado no mesmo curso: permanece em `4_semestre` e `2_ano_superior`;
2. estudante superior em `4_semestre` é desvinculado e reingressa em outro curso superior: novo vínculo começa em `1_semestre` e `1_ano_superior`, mantendo histórico anterior;
3. estudante médio em `2_ano_medio` é interrompido e retorna ao mesmo curso: permanece em `2_ano_medio`;
4. estudante médio em `2_ano_medio` muda para outro curso médio: novo vínculo começa em `1_ano_medio`, mantendo histórico anterior;
5. estudante fundamental inativo e depois reativado sem mudança acadêmica: não volta automaticamente para `1_ano_fundamental`;
6. replay de eventos mantém o mesmo estado final da projeção.

### 5. Mensagens de erro e auditoria

Quando uma operação tentar resetar progressão sem evento de mudança de curso ou comando explícito permitido, retornar erro claro, por exemplo:

```json
{
  "error": "Progressão acadêmica não pode ser reiniciada por inativação, desvínculo ou reativação. Informe mudança de curso ou comando explícito de reinício acadêmico."
}
```

Registrar auditoria suficiente para responder:

- qual evento alterou o vínculo;
- qual era a posição acadêmica antes da inatividade/desvínculo;
- qual posição foi restaurada no retorno;
- se houve mudança de curso, qual curso anterior e qual novo curso;
- quem executou a operação e quando.

## Regras de segurança e consistência

- Não apagar eventos históricos do event store.
- Não sobrescrever notas, faltas, avaliações, turmas, sumários, aprovações/reprovações ou documentos acadêmicos anteriores.
- Não recalcular ledger, mensalidades, propinas, cobranças ou pagamentos históricos como consequência de inativação/desvínculo/revínculo.
- Não usar status ativo/inativo como fonte para determinar ano/semestre inicial.
- Não criar migração destrutiva que zere campos acadêmicos existentes.
- Garantir idempotência para reprocessamento de eventos e reconstrução de projeções.
- Manter compatibilidade com estudantes antigos que ainda não possuam todos os campos de histórico; nesses casos, aplicar fallback seguro e documentado.

## Critérios de aceite

- Inativar, interromper, trancar ou desvincular estudante não reseta histórico nem progressão.
- Reativar/revincular/reingressar no mesmo curso mantém o ano/semestre anterior.
- Mudar de curso superior posiciona o vínculo atual em `1_semestre`/`1_ano_superior` do novo curso, preservando histórico anterior.
- Mudar de curso médio posiciona o vínculo atual em `1_ano_medio` do novo curso, preservando histórico anterior.
- O comportamento é comprovado por testes automatizados e por replay de eventos/projeções quando aplicável.
- Nenhuma operação de status/vínculo causa exclusão ou reescrita destrutiva de dados acadêmicos, financeiros, ledger ou auditoria.
