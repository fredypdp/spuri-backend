---
modificado: 2026-06-30 00:00
criado: 2026-06-30 00:00
---
# Corrigir avaliação final automática por matéria, regras por nível e pendências

## Objetivo

Corrigir o sistema de **avaliação final automática** para que ele calcule e registre a `nota_final` de **cada matéria disciplinar aplicável**, em vez de tratar a avaliação como uma nota final única ou como um cálculo genérico sobre todas as notas do estudante.

A avaliação final deve passar a executar a cadeia de regras configurada pela academia de forma segura, auditável e compatível com os cenários reais de ensino **fundamental**, **médio** e **superior**, incluindo recuperação por regra descendente e pendências de matérias quando permitido.

Essa mudança também deve renomear o campo público da regra raiz de `tipo_ensino` para `nivel`, remover totalmente o nome antigo e introduzir o recurso de **matérias pendentes**.

## Contexto e motivação

Atualmente, a avaliação final automática não está calculando corretamente a nota final por matéria. O comportamento esperado é que a regra seja aplicada individualmente às matérias disciplinares do escopo correto do estudante, curso, ano ou período, e que a aprovação seja decidida a partir do conjunto de resultados por matéria.

O sistema precisa suportar:

- aprovação normal quando o estudante atinge a nota mínima nas matérias exigidas;
- recuperação quando uma matéria reprovada aciona uma regra descendente;
- aprovação com pendência em níveis que permitem cadeira em atraso;
- bloqueio de progressão ou conclusão quando a pendência está no nível de conclusão;
- regularização posterior de matérias pendentes com evento auditável próprio.

Como ainda não existem regras criadas em produção por academias, não deve haver compatibilidade retroativa com o contrato antigo de `tipo_ensino`.

## Regra de negócio a implementar

### Renomeação de `tipo_ensino` para `nivel`

- O campo público da regra de avaliação final deve se chamar obrigatoriamente `nivel`.
- O campo antigo `tipo_ensino` deve ser removido do contrato público e interno da nova regra.
- Não deve existir alias, fallback, compatibilidade, aceitação silenciosa, tradução automática, duplicação no payload ou suporte legado para `tipo_ensino`.
- Payloads contendo `tipo_ensino` devem falhar com erro de validação claro.
- O frontend deve obrigatoriamente passar a usar `nivel` quando precisar informar o nível.
- Toda regra de avaliação final deve ter `nivel` definido.
- O backend deve preencher `nivel` automaticamente a partir do nível da academia autenticada, exceto quando a academia for mista.
- Para academia mista, o payload da regra pode enviar apenas `nivel='fundamental'` ou `nivel='medio'`.
- Não deve ser possível criar regra com `nivel` incompatível com o cadastro da academia.
- O banco, DTOs, validações, handlers, documentação e testes devem usar a nova nomenclatura de forma consistente.

### Escopo de `anos_academicos`

- O campo `anos_academicos` deve estar disponível apenas para regras com `nivel='fundamental'`.
- Regras de `nivel='medio'` e `nivel='superior'` não devem aceitar `anos_academicos` no payload público.
- Para `nivel='medio'`, o escopo acadêmico deve ser resolvido pelo `ano_escolar_medio` do estudante e pelas matérias disciplinares do curso da matéria avaliada.
- Para `nivel='superior'`, o escopo acadêmico deve ser resolvido pelo `periodo` da matéria e pelo período/semestre atual do estudante.

### Fórmula por nível

- A fórmula continua sendo declarativa e validada pelo parser próprio do backend.
- Para `nivel='fundamental'` e `nivel='medio'`, a fórmula pode referenciar notas no formato com categoria e período, conforme contrato existente.
- Para `nivel='superior'`, a parte de período da fórmula (`[categoria de nota, período]`) não deve existir no payload da regra.
- Em `nivel='superior'`, o backend deve preencher automaticamente o período da fórmula usando o `periodo` da matéria disciplinar avaliada.
- O backend não deve exigir que a regra superior declare manualmente o semestre/período dentro da fórmula.
- Validações, extração de categorias envolvidas e carregamento de notas devem considerar a diferença entre fórmula com período explícito e fórmula superior com período inferido.

## Execução da regra raiz

### `nivel='fundamental'`

- A regra raiz deve ser aplicada a cada matéria disciplinar da academia correspondente ao `ano_escolar_fundamental` do estudante avaliado.
- Para cada matéria aplicável, o backend deve calcular uma `nota_final` própria.
- O estudante só deve ser aprovado se a `nota_final` for maior ou igual à `nota_minima_aprovacao` em todas as matérias avaliadas.
- Se qualquer matéria ficar abaixo da nota mínima, a cadeia descendente aplicável deve ser avaliada conforme as regras descendentes configuradas.

### `nivel='medio'`

- A regra raiz deve ser aplicada a cada matéria disciplinar do curso da matéria avaliada, usando o `curso_id` da matéria.
- O escopo deve corresponder ao `ano_escolar_medio` do estudante.
- Para cada matéria aplicável, o backend deve calcular uma `nota_final` própria.
- Regras de `nivel='medio'` devem incluir o campo `materias_chave`, que é um array de IDs de matérias disciplinares obrigatórias.
- O estudante só deve ser aprovado diretamente se a `nota_final` de todas as matérias em `materias_chave` for maior ou igual à `nota_minima_aprovacao`.
- Quando uma matéria ficar abaixo da nota mínima, o backend deve acionar a regra descendente aplicável àquela matéria, se existir.
- `materias_chave` deve aceitar apenas matérias disciplinares válidas, ativas, pertencentes ao curso e ano escolar médio aplicável.

### `nivel='superior'`

- A regra raiz deve ser aplicada a cada matéria disciplinar do curso da matéria avaliada, usando o `curso_id` da matéria.
- O escopo deve corresponder ao `periodo` ou semestre atual do estudante.
- Para cada matéria aplicável, o backend deve calcular uma `nota_final` própria.
- O estudante só deve ser aprovado se a `nota_final` for maior ou igual à `nota_minima_aprovacao` em todas as matérias avaliadas.
- Na fórmula superior, o período deve ser inferido automaticamente a partir do `periodo` da matéria em avaliação.

## Registro/evento de avaliação final

- O evento de avaliação final deve registrar a lista completa de matérias avaliadas.
- Cada item da lista deve conter, no mínimo:
  - `materia_id`;
  - `nota_final`;
  - indicação de aprovado/reprovado naquela matéria, quando útil para auditoria;
  - regra usada para o cálculo, quando diferente por etapa da cadeia;
  - dados suficientes para reconstituir o cálculo via snapshot.
- O evento de aprovação/reprovação final do estudante deve continuar sendo auditável e idempotente.
- A decisão geral do estudante deve derivar do conjunto de resultados por matéria e das regras de pendência, não de uma única média global.
- A resposta da API e as projeções devem expor o conjunto `materia_id` + `nota_final` para todas as matérias avaliadas.

## Regras descendentes

- Regras descendentes continuam representando etapas de recuperação, recurso, exame ou qualquer avaliação posterior à regra raiz.
- Uma regra descendente pode definir a lista de matérias às quais ela se aplica.
- A lista de matérias da regra descendente deve ser um subconjunto das matérias avaliadas pela regra ascendente correspondente.
- A regra descendente só deve ser acionada para matérias em que o estudante ficou abaixo da nota mínima na regra ascendente que desperta essa descendente.
- A aprovação ou reprovação na regra descendente deve ser calculada por matéria.
- Se uma matéria não estiver na lista de aplicação da regra descendente, ela não deve ser recalculada por essa regra.
- Não deve ser possível criar regra descendente órfã, cíclica, apontando para nível incompatível ou para matérias fora do escopo da regra ascendente.
- A cadeia deve continuar tendo uma única regra raiz aplicável por escopo.

## Pendências de matérias

### Escopo

- Pendências são permitidas apenas para regras de `nivel='medio'` e `nivel='superior'`.
- Regras de `nivel='fundamental'` não devem permitir pendências.
- A pendência só deve ser considerada depois que o estudante reprovar na regra raiz e em todas as regras descendentes aplicáveis.
- Se não existir regra descendente, a avaliação de pendência deve ocorrer após a regra raiz.

### `pendencia_permitida`

- Matérias com `pendencia_permitida=true` podem gerar pendência quando o estudante não atinge a nota mínima após esgotar a cadeia de avaliação final.
- Matérias com `pendencia_permitida=false` devem reprovar o estudante normalmente se a nota mínima não for atingida.
- O backend deve respeitar o campo da matéria no momento do cálculo e registrar o snapshot necessário no evento.

### `limite_materias_pendentes`

- Toda regra de `nivel='medio'` ou `nivel='superior'` deve ter o campo inteiro `limite_materias_pendentes`.
- `limite_materias_pendentes` representa o número máximo de matérias em que o estudante pode ficar abaixo da nota mínima e ainda assim ser aprovado/progredir com pendência.
- O limite deve ser aplicado ao resultado final da última regra da cadeia aplicável.
- Se o número de matérias abaixo da nota mínima for menor ou igual ao limite e todas permitirem pendência, o estudante pode aprovar com pendência.
- Se o número de matérias abaixo da nota mínima for maior que o limite, o estudante deve reprovar, mas as pendências devem ser criadas para todas as matérias elegíveis conforme a regra de negócio.
- O limite não deve contar matérias que não permitem pendência como elegíveis para aprovação com pendência.
- Valores negativos devem ser rejeitados.
- Ausência do campo em `nivel='medio'` ou `nivel='superior'` deve falhar na criação/edição da regra.

### Novo recurso `materias pendentes`

Criar um novo recurso persistente para armazenar matérias pendentes do estudante.

Cada registro deve conter, no mínimo:

- identificador único;
- identificação do estudante;
- identificação da matéria;
- identificação da academia;
- identificação do curso quando aplicável;
- nível (`medio` ou `superior`);
- ano escolar médio ou período/semestre superior relacionado;
- ano letivo da avaliação que gerou a pendência;
- regra e evento de avaliação final que geraram a pendência;
- `pendente` booleano, com `true` quando a pendência ainda está aberta;
- timestamps e metadados necessários para auditoria.

Regras do recurso:

- Não deve criar pendência duplicada aberta para o mesmo estudante, matéria, academia e escopo letivo.
- Deve permitir consultar pendências abertas e históricas.
- Deve permitir identificar rapidamente se o estudante ainda possui pendências antes de progressão, conclusão ou finalização de ciclo.
- A criação e baixa da pendência devem ocorrer por eventos auditáveis, não por atualização silenciosa.

## Bloqueio por `pendencia_nivel_conclusao`

- A matéria possui o campo `pendencia_nivel_conclusao`.
- Esse campo impede que o estudante finalize o ciclo ou siga o processo normal quando possui pendência em etapa de conclusão.
- Em `nivel='medio'`, se `pendencia_nivel_conclusao` for igual ao `ano_escolar_medio` atual do estudante, ele pode ser aprovado com pendência, mas deve ser impedido de seguir automaticamente para o próximo ano ou finalizar o médio.
- Em `nivel='superior'`, se `pendencia_nivel_conclusao` for igual ao `semestre_atual` do estudante, ele pode ser aprovado com pendência, mas deve ser impedido de seguir automaticamente para o próximo semestre ou finalizar o superior.
- Enquanto houver pendência bloqueante, o sistema deve deixar o estudante em estado que permita regularização, sem executar o fluxo normal de progressão/conclusão.
- Quando todas as pendências bloqueantes forem baixadas, o sistema deve seguir automaticamente o processo normal adequado ao nível atual.

## Avaliação de matérias pendentes

- O novo recurso de matérias pendentes deve permitir lançar uma avaliação final específica para regularização de pendências.
- Essa avaliação deve gerar evento auditável próprio.
- Para cada matéria pendente avaliada, o evento deve registrar:
  - dados da pendência previamente armazenada;
  - `materia_id`;
  - nota obtida na regularização;
  - `aprovado` booleano;
  - regra, critério ou configuração usada para decidir a aprovação, quando aplicável;
  - operador/ator que lançou a avaliação, quando disponível.
- Quando o estudante aprovar na matéria pendente, o campo `pendente` do registro deve passar para `false` por projeção/evento.
- Quando o estudante reprovar na regularização, a pendência deve continuar com `pendente=true`.
- Quando o estudante não tiver mais matérias pendentes abertas, o sistema deve retomar automaticamente o fluxo normal adequado ao nível:
  - avanço para o próximo ano no médio, quando cabível;
  - avanço para o próximo período/semestre no superior, quando cabível;
  - conclusão/finalização do ciclo, quando cabível.

## Contrato de API e validações

### Criação/edição de regra

- `POST /academia/avaliacao-final/regras` e `PUT /academia/avaliacao-final/regras/:id` devem refletir o novo contrato.
- `nivel` deve substituir `tipo_ensino` em payload, resposta, filtros, DTOs, documentação e testes.
- `anos_academicos` só deve ser aceito quando `nivel='fundamental'`.
- `materias_chave` deve ser obrigatório para `nivel='medio'` em regra raiz.
- `limite_materias_pendentes` deve ser obrigatório para `nivel='medio'` e `nivel='superior'`.
- Regras descendentes podem informar a lista de matérias às quais se aplicam.
- Campos incompatíveis com o nível devem gerar erro de validação claro.
- Payloads com campos legados ou ambíguos devem ser rejeitados.

### Listagem e leitura

- As respostas devem expor `nivel`, nunca `tipo_ensino`.
- Regras de `nivel='fundamental'` podem expor `anos_academicos`.
- Regras de `nivel='medio'` e `nivel='superior'` não devem expor `anos_academicos` como campo de configuração pública.
- Regras de `nivel='medio'` devem expor `materias_chave`.
- Regras de `nivel='medio'` e `nivel='superior'` devem expor `limite_materias_pendentes`.

### Execução automática

- A execução automática por lançamento de nota deve descobrir as matérias afetadas e recalcular somente o escopo necessário.
- A avaliação não deve fechar se faltarem notas exigidas pela fórmula para qualquer matéria obrigatória do escopo.
- Uma avaliação já registrada deve continuar protegida contra duplicidade idempotente no mesmo escopo, ano letivo, nível, regra e estudante.
- A decisão deve usar o conjunto final de resultados por matéria, incluindo recuperação e pendências.

## Persistência e migrações

- Criar migrações para renomear/remodelar `tipo_ensino` para `nivel` onde necessário.
- Como ainda não existem regras criadas por academias, não é necessário manter compatibilidade de dados legados de regras.
- Criar colunas/estruturas para:
  - resultados por matéria no evento/projeção de avaliação final;
  - `materias_chave` nas regras de nível médio;
  - lista de matérias aplicáveis em regras descendentes;
  - `limite_materias_pendentes` em regras de médio/superior;
  - novo recurso/projeção/tabela de matérias pendentes.
- Índices devem garantir consulta eficiente por academia, estudante, matéria, nível, ano letivo, status de pendência e escopo acadêmico.
- Restrições devem impedir duplicidade de pendência aberta para o mesmo estudante/matéria/escopo.

## Segurança e auditoria

- Toda decisão de aprovação, reprovação, recuperação, pendência e baixa de pendência deve ser rastreável por evento.
- Snapshots de regra, fórmula, notas finais por matéria e configuração de pendência devem ser preservados.
- Alterações futuras em regras ou matérias não devem alterar avaliações já registradas.
- O backend não deve usar cálculo dinâmico inseguro, `eval`, SQL dinâmico com fórmula de usuário ou qualquer execução de código fornecido no payload.
- Erros devem ser estruturados, claros e sem vazamento de dados internos.
- Escritas devem usar a academia autenticada e nunca aceitar `codigo_academia` de outro contexto para alterar dados.

## Documentação obrigatória

Atualizar a documentação do sistema e da API para explicar:

- o novo campo `nivel`;
- a remoção total de `tipo_ensino`;
- regras por nível;
- fórmula superior com período inferido;
- cálculo de nota final por matéria;
- `materias_chave` no médio;
- regras descendentes por matéria;
- pendência permitida;
- `limite_materias_pendentes`;
- recurso de matérias pendentes;
- avaliação/regularização de matérias pendentes;
- bloqueio por `pendencia_nivel_conclusao`.

## Critérios de aceite

- Criar regra com `tipo_ensino` falha; criar com `nivel` funciona conforme o nível da academia.
- Academia não mista não consegue criar regra para nível incompatível.
- Academia mista consegue criar regra apenas para `fundamental` ou `medio`.
- `anos_academicos` é aceito apenas em regra fundamental.
- Regra superior rejeita fórmula com período explícito e calcula usando o período da matéria.
- Regra fundamental calcula `nota_final` por matéria disciplinar do ano fundamental do estudante.
- Regra média calcula `nota_final` por matéria disciplinar do curso/ano médio e usa `materias_chave` para decisão de aprovação direta.
- Regra superior calcula `nota_final` por matéria disciplinar do curso/período do estudante.
- Evento de avaliação final registra todas as matérias avaliadas com `materia_id` e `nota_final`.
- Regra descendente executa apenas para matérias reprovadas e configuradas no seu escopo.
- Pendências são criadas apenas para médio/superior e apenas depois de esgotar a cadeia aplicável.
- `limite_materias_pendentes` controla corretamente aprovação com pendência.
- Matéria sem `pendencia_permitida` reprova o estudante quando fica abaixo da nota mínima.
- Pendência bloqueante por `pendencia_nivel_conclusao` impede progressão ou conclusão automática.
- Avaliação de matéria pendente baixa `pendente` para `false` quando aprovada.
- Quando não restam pendências abertas, o sistema retoma automaticamente a progressão ou conclusão adequada.
- Não há referência ativa ao contrato antigo `tipo_ensino` em regras de avaliação final, exceto em migrações históricas inevitáveis ou notas de remoção documentadas.

## Testes obrigatórios

- Testes unitários do parser/validador de fórmula para os três níveis.
- Testes de criação, edição, listagem e deleção lógica de regras com `nivel`.
- Testes garantindo rejeição absoluta de `tipo_ensino` no contrato público novo.
- Testes de avaliação automática fundamental por matéria.
- Testes de avaliação automática média com `materias_chave`.
- Testes de avaliação automática superior com período inferido pela matéria.
- Testes de cadeia descendente por matéria.
- Testes de pendência permitida e não permitida.
- Testes de `limite_materias_pendentes` com limite zero, dentro do limite e acima do limite.
- Testes de bloqueio por `pendencia_nivel_conclusao`.
- Testes de avaliação e baixa de matérias pendentes.
- Testes de idempotência para avaliação final e pendências abertas.
- Testes de documentação/contrato para garantir que respostas públicas usam `nivel` e não `tipo_ensino`.
