# Regra de negócio: estudante aprovado sem ano académico seguinte na academia

## Objetivo
Definir o comportamento do sistema quando uma avaliação final aprova um estudante, mas a academia não oferta ou ainda não cadastrou o ano académico seguinte necessário para a progressão.

## Contexto atual
- A avaliação final já é tratada como decisão única por estudante e ano letivo.
- Para avaliações escolares, a remoção automática da turma foi substituída por progressão/retenção de turma.
- Hoje a documentação indica fallback para turma ativa do próximo ano, mas não especifica o que ocorre quando o próximo ano académico não existe na academia.

## Regra principal
Ao aprovar um estudante, o sistema deve calcular o próximo ano académico da trajetória avaliada. Se esse próximo ano não existir na configuração/oferta da academia, o estudante não deve ser movido automaticamente para uma turma inexistente e a aprovação deve ser registrada com um estado operacional explícito.

## Cenários esperados

### 1. Conclusão natural de ciclo
Quando o estudante está no último ano de um ciclo ofertado pela plataforma:
- 9º ano fundamental aprovado: marcar `status_escolar_fundamental = finalizado`.
- 12º/13º ano médio aprovado, conforme modelo usado pelo sistema: marcar `status_escolar_medio = finalizado`.
- Último ano/semestre superior aprovado: marcar `status_superior = finalizado` ou status equivalente de conclusão.
- Não exigir ano académico seguinte.
- Não mover para turma nova.
- Registrar no evento/projeção a conclusão do ciclo e a academia de origem.

### 2. Academia não cadastrou o próximo ano, mas o ciclo ainda não terminou
Exemplo: estudante no `4_ano_fundamental`, academia oferta apenas até `4_ano_fundamental`, e o próximo ano esperado seria `5_ano_fundamental`.

Comportamento recomendado:
- Permitir registrar a aprovação, porque a decisão pedagógica aconteceu.
- Não alterar o ano académico atual para um valor que a academia não oferta.
- Não remover o estudante da turma atual.
- Definir um marcador de pendência, por exemplo `progressao_pendente`.
- Registrar motivo: `ano_academico_seguinte_nao_ofertado`.
- Expor essa pendência em listagens e detalhes do estudante.
- Bloquear nova matrícula acadêmica interna para o ano letivo seguinte até que a academia resolva a pendência.

### 3. Academia cadastrou o próximo ano, mas não existe turma compatível
- A aprovação deve ser registrada.
- Atualizar o ano académico do estudante para o próximo ano apenas se a regra operacional aceitar estudante sem turma temporariamente.
- Preferência: criar pendência `turma_destino_nao_encontrada` e manter vínculo histórico na turma anterior, sem inserir em uma turma incompatível.
- Permitir que a academia resolva manualmente escolhendo/criando turma destino.

### 4. Transferência para outra instituição
Se a academia não oferta o ano seguinte, o sistema deve permitir que o estudante solicite matrícula ou transferência para outra instituição que oferte o ano necessário, mantendo o histórico da aprovação na instituição anterior.

## Validações necessárias
- Validar o tipo de ensino da avaliação: fundamental, médio ou superior.
- Validar o ano académico atual do estudante no momento da avaliação.
- Validar se a avaliação pertence ao ano letivo ativo/finalizável da academia.
- Validar se já existe avaliação final para o mesmo estudante, tipo e ano letivo.
- Calcular o próximo ano por tabela canônica, nunca por concatenação textual.
- Verificar se o próximo ano pertence a `anos_academicos` da academia ou ao curso aplicável.
- Verificar turma destino apenas entre turmas ativas, da mesma academia, mesmo nível/curso, mesmo ano académico e ano letivo aplicável.

## Fluxo operacional
1. Academia registra avaliação final do estudante.
2. Sistema carrega estudante, academia, curso/turma atual e ano letivo ativo.
3. Sistema valida idempotência da avaliação.
4. Sistema calcula resultado: aprovado ou reprovado.
5. Se reprovado: estudante permanece no mesmo ano/turma, com retenção registrada.
6. Se aprovado: sistema calcula próximo ano académico.
7. Sistema verifica se o próximo ano existe na academia/curso.
8. Se existir, tenta resolver turma destino.
9. Se não existir ou não houver turma, grava aprovação com pendência operacional.
10. Pendência aparece em painel da academia e em consultas administrativas.

## Modelo de dados sugerido
Adicionar campos/eventos que permitam rastrear a pendência sem apagar histórico:
- `progressao_status`: `concluida`, `pendente`, `nao_aplicavel`.
- `progressao_motivo`: `ano_academico_seguinte_nao_ofertado`, `turma_destino_nao_encontrada`, `fim_de_ciclo`.
- `ano_academico_origem`.
- `ano_academico_destino_previsto`.
- `turma_origem`.
- `turma_destino`, opcional.
- `resolvido_por`, `resolvido_em`, opcional.

## Endpoints sugeridos
- `GET /academia/progressoes-pendentes`
- `POST /academia/progressoes-pendentes/:id/resolver`
- `GET /estudantes/:codigo/progressoes`

## Critérios de aceite
- Aprovação no fim natural de ciclo finaliza o ciclo sem erro.
- Aprovação sem próximo ano ofertado cria pendência rastreável.
- O sistema não cria turma automaticamente sem solicitação explícita.
- O sistema nunca move estudante para ano/turma incompatível.
- A pendência pode ser resolvida posteriormente por criação de turma, configuração do ano académico ou transferência.
