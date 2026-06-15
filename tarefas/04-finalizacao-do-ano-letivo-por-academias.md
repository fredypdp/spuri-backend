# Finalização do ano letivo por academias

## Objetivo
Criar funcionalidade para que academias declarem a finalização de um ano letivo, permitindo que a plataforma impeça regressão para anos letivos anteriores quando todas as academias elegíveis já finalizaram determinado ano.

## Problema a resolver
A plataforma pode definir mal o ano letivo global se permitir selecionar um ano anterior ao último ano totalmente finalizado pelas academias. A finalização por academia cria um marco operacional verificável.

## Regra principal
Se todas as academias aplicáveis finalizaram o ano letivo `X`, o Admin FPP não pode definir o ano letivo global como `X` ou anterior. O próximo ano global permitido deve ser `X + 1`.

## Quem pode finalizar
- Academia ativa pode finalizar o seu próprio ano letivo.
- Admin FPP pode consultar e, em casos excepcionais, corrigir/reabrir com justificativa auditada.

## Pré-condições para academia finalizar
- Ano letivo ativo configurado na academia.
- Não existirem avaliações finais pendentes obrigatórias, conforme regra da academia.
- Não existirem notas/faltas em processamento assíncrono para o ano.
- Não existirem progressões pendentes críticas sem decisão.
- Turmas do ano letivo devem ter estado consistente.
- Períodos oficiais globais devem estar definidos.

## Fluxo operacional da academia
1. Academia acessa `Finalizar ano letivo`.
2. Sistema executa checklist de pendências.
3. Se houver pendências, mostra lista e bloqueia finalização ou permite exceção configurada.
4. Academia confirma encerramento.
5. Sistema grava evento `AnoLetivoAcademiaFinalizado`.
6. Projeção marca `codigo_academia`, `ano_letivo`, `finalizado_em`, `finalizado_por`.
7. Operações de notas/faltas/avaliação para aquele ano passam a ser bloqueadas, exceto por reabertura auditada.

## Fluxo operacional do Admin FPP
1. Admin FPP consulta painel global de anos letivos.
2. Sistema mostra academias finalizadas, pendentes e inativas.
3. Ao definir novo ano letivo global, sistema calcula último ano finalizado por todas as academias elegíveis.
4. Se Admin tentar definir ano anterior ou igual ao último finalizado globalmente, bloquear.
5. Admin só pode definir o próximo ano permitido ou superior, conforme política.

## Academias elegíveis
Definir claramente quem entra no cálculo:
- Academias ativas no período do ano letivo.
- Academias suspensas podem contar como pendentes ou serem excluídas com justificativa Admin FPP.
- Academias criadas após o fim do ano letivo não devem bloquear a finalização histórica.
- Academias desativadas antes do início do ano letivo não devem entrar no cálculo.

## Modelo de dados sugerido
- `projection_academia_anos_letivos_finalizados`:
  - `codigo_academia`.
  - `ano_letivo`.
  - `tipo_ensino`.
  - `status`: `aberto`, `finalizado`, `reaberto`.
  - `finalizado_por`.
  - `finalizado_em`.
  - `reaberto_por`, `reaberto_em`, `motivo_reabertura`.
- `projection_sistema_config` deve expor o menor ano global permitido.

## Validações
- Uma academia não pode finalizar ano letivo diferente do seu ativo ou do ano em processo de fechamento autorizado.
- Ano finalizado não aceita novos lançamentos comuns.
- Reabertura exige Admin FPP e justificativa.
- Não permitir finalização duplicada sem idempotência.
- O cálculo global deve considerar tipo de ensino escolar e superior separadamente se os calendários forem independentes.

## Critérios de aceite
- Academia consegue finalizar ano letivo após checklist positivo.
- Ano letivo finalizado bloqueia lançamentos comuns retroativos.
- Admin FPP visualiza status de finalização por academia.
- Sistema impede definir ano global anterior ou igual ao último finalizado por todas as academias elegíveis.
- Toda finalização/reabertura gera evento auditável.
