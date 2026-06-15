# Situação prevista: estudante matriculado em mais de uma instituição

## Objetivo
Definir como o sistema deve lidar quando um mesmo estudante se matricula em mais de uma instituição, evitando duplicidade indevida e permitindo cenários legítimos.

## Princípio geral
O estudante deve ser uma entidade global identificável por documentos e código, enquanto os vínculos com instituições devem ser entidades separadas. Um estudante pode ter múltiplos vínculos, mas as regras devem impedir conflitos acadêmicos.

## Cenários possíveis

### Matrícula simultânea indevida
Exemplo: estudante ativo no mesmo nível, mesmo ano letivo e mesmo turno em duas escolas diferentes.
- Deve ser bloqueada ou exigir autorização administrativa.
- Sistema deve informar a instituição onde já existe vínculo ativo, respeitando privacidade.

### Matrícula em níveis diferentes
Exemplo: estudante concluiu médio em uma instituição e está no superior em outra.
- Deve ser permitida se não houver conflito de status.
- Histórico escolar anterior deve permanecer na instituição de origem.

### Transferência
Exemplo: estudante sai de uma escola e entra em outra no mesmo ano letivo.
- Deve haver fluxo explícito de transferência.
- Vínculo anterior deve ser encerrado como `transferido`, não apagado.
- Nova academia deve receber histórico mínimo necessário.

### Cursos independentes no superior
Exemplo: estudante cursa dois cursos superiores em instituições diferentes.
- Pode ser permitido pela regra global ou configuração do Admin FPP.
- Deve validar conflito de calendário/turno apenas se o produto exigir.

### Solicitação duplicada pendente
Exemplo: responsável envia solicitações para várias academias.
- Sistema pode permitir várias solicitações pendentes.
- Ao aprovar uma, as demais podem permanecer pendentes, ser sinalizadas ou exigir decisão manual.

## Regra de negócio proposta
- `projection_estudantes` deve representar a pessoa/conta do estudante.
- Criar ou fortalecer entidade `matricula_vinculo` por academia, ano letivo, nível, curso e status.
- Um estudante não pode ter dois vínculos ativos conflitantes no mesmo `tipo_ensino`, `ano_academico` e `ano_letivo`, salvo exceção autorizada.
- Histórico de vínculos deve ser preservado para consultas e certificados.

## Status de vínculo sugeridos
- `solicitado`.
- `ativo`.
- `transferido`.
- `trancado`.
- `concluido`.
- `cancelado`.
- `indeferido`.

## Validações
- B.I. principal do estudante deve ser único globalmente quando informado.
- Se estudante não tem B.I., usar combinação de dados com alerta de possível duplicidade, não como chave definitiva.
- Antes de aprovar matrícula, consultar vínculos ativos globais.
- Bloquear vínculo conflitante por padrão.
- Permitir exceção apenas com perfil autorizado e justificativa.
- Transferência exige encerramento do vínculo anterior ou aceite explícito de coexistência permitida.

## Fluxo operacional: nova matrícula com possível duplicidade
1. Academia tenta cadastrar ou aprovar solicitação.
2. Sistema busca estudante por B.I. e outros identificadores.
3. Se encontrar estudante global, sistema não cria pessoa duplicada; cria nova solicitação/vínculo.
4. Sistema verifica vínculos ativos conflitantes.
5. Se houver conflito, bloqueia ou envia para análise.
6. Se não houver conflito, ativa vínculo e registra evento.

## Fluxo operacional: transferência
1. Academia destino inicia solicitação de transferência.
2. Sistema identifica vínculo ativo na academia origem.
3. Academia origem ou Admin FPP confirma transferência, conforme regra.
4. Sistema encerra vínculo origem com motivo `transferencia`.
5. Sistema ativa vínculo destino.
6. Histórico acadêmico continua consultável.

## Impactos técnicos
- Separar conta/pessoa de vínculo acadêmico.
- Ajustar consultas para filtrar por vínculo atual da academia autenticada.
- Garantir que notas, faltas, turmas e avaliações apontem para o vínculo correto, não apenas para o estudante global.
- Eventos devem diferenciar `EstudanteCriado` de `VinculoAcademicoCriado`.

## Critérios de aceite
- Não é criada pessoa duplicada para mesmo B.I.
- Sistema detecta vínculo ativo conflitante antes de aprovar matrícula.
- Transferência mantém histórico anterior.
- Consultas da academia mostram apenas vínculos sob sua responsabilidade.
- Admin FPP consegue auditar todas as instituições vinculadas a um estudante.
