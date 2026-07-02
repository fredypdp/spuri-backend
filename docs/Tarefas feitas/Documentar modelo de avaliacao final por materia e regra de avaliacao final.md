---
modificado: 2026-07-01 00:00
criado: 2026-07-01 00:00
---
# Documentar modelo de avaliação final por matéria e regra de avaliação final

## Objetivo

Preciso que você faça uma atualização documental crítica, completa e aprofundada em `docs/Spuri - Documentação.md`, com foco principal na seção **"5. Processos de Negócio"**, para descrever o modelo atualmente suportado pelo sistema para:

1. **avaliação final automática por matéria**;
2. **montagem/criação de regras de avaliação final**;
3. **execução de regras descendentes**;
4. **aprovação, reprovação e aprovação com pendências**;
5. **criação, bloqueio, consulta e regularização de matérias pendentes**.

A implementação já foi realizada conforme a tarefa `tarefas/Corrigir regra de avaliacao final automatica por materia e pendencias.md` e está resumida em `tarefas/Resumo - Corrigir regra de avaliacao final automatica por materia e pendencias.md`. Seu trabalho principal não é redesenhar a regra de negócio, mas sim **transformar o comportamento implementado em documentação funcional detalhada, clara, auditável e útil para produto, backend, frontend, QA e operação**.

A documentação deve explicar o máximo de cenários possível suportados atualmente pelo sistema, especialmente em torno de avaliação final e regra de avaliação final. Essa é a prioridade absoluta da tarefa.

## Arquivos de referência obrigatórios

Leia, compare e use como base, no mínimo:

- `tarefas/Corrigir regra de avaliacao final automatica por materia e pendencias.md`;
- `tarefas/Resumo - Corrigir regra de avaliacao final automatica por materia e pendencias.md`;
- `tarefas/Corrigir e depurar regra de avaliação final.md`;
- `docs/Spuri - Documentação.md`;
- `docs/Spuri - API.md`;
- handlers, agregados, projeções, migrações e testes relacionados a avaliação final, regras de avaliação final, matérias disciplinares, notas, progressão, pendências e estudantes.

Não trate os arquivos de tarefa como documentação final por si só. Eles devem orientar a investigação, mas a documentação final precisa refletir o que o código realmente suporta neste momento.

## Forma de atuação esperada

Atue como engenheiro sênior responsável por documentação funcional de uma feature crítica. Faça uma leitura investigativa do código antes de escrever a documentação, para evitar documentar comportamento desejado que não esteja implementado.

Você deve:

1. identificar o comportamento real implementado;
2. mapear os cenários de negócio suportados;
3. apontar diferenças relevantes entre intenção da tarefa e comportamento atual, caso encontre;
4. atualizar `docs/Spuri - Documentação.md` com linguagem funcional, estruturada e precisa;
5. manter coerência com `docs/Spuri - API.md`, sem transformar a documentação funcional em mera cópia da documentação de endpoints;
6. preservar a terminologia nova baseada em `nivel`, não em `tipo_ensino`.

## Prioridade principal da documentação

A prioridade principal é aprofundar a seção **"5. Processos de Negócio"** de `docs/Spuri - Documentação.md`, reorganizando ou expandindo o conteúdo existente de avaliação final para explicar o modelo atual.

Se já existirem trechos antigos, simplificados, duplicados ou conflitantes sobre avaliação final, regras antigas, `tipo_ensino`, média global, avaliação manual ou escopo por `anos_academicos` no superior/médio, eles devem ser corrigidos, substituídos ou recontextualizados.

A documentação deve deixar claro que a avaliação final automática não é mais uma nota global única do estudante: ela é calculada por matéria disciplinar aplicável, e a decisão geral deriva do conjunto de resultados por matéria, da cadeia de regras e das regras de pendência.

## Conteúdo mínimo obrigatório

### 1. Visão geral do novo modelo de avaliação final

Documente que:

- a avaliação final é automática e auditável;
- a execução é disparada pelo fluxo de lançamento de notas quando o sistema identifica que há notas suficientes para aplicar a regra;
- a academia configura regras de avaliação final, mas não envia manualmente a nota final calculada;
- a `nota_final` é calculada por fórmula declarativa;
- o cálculo ocorre por matéria disciplinar aplicável;
- o resultado geral do estudante deriva dos resultados de cada matéria;
- a avaliação registrada é idempotente no escopo suportado;
- eventos/projeções preservam snapshots suficientes para auditoria.

Explique também os principais conceitos envolvidos:

- regra raiz;
- regra descendente;
- `type` da regra;
- `nivel` da regra;
- fórmula;
- `nota_minima_aprovacao`;
- matérias avaliadas;
- `materias_chave`;
- `materias_aplicaveis`;
- `limite_materias_pendentes`;
- `pendencia_permitida`;
- `pendencia_nivel_conclusao`;
- matérias pendentes.

### 2. Montagem/criação de regras de avaliação final

Descreva, em profundidade, como uma academia deve montar uma regra de avaliação final.

A documentação deve explicar:

- o uso obrigatório de `nivel` no contrato público;
- a remoção de `tipo_ensino` do contrato novo;
- como o backend preenche ou valida `nivel` pela academia autenticada;
- o comportamento para academia fundamental, média, superior e mista;
- quando `anos_academicos` é permitido;
- quando `anos_academicos` deve ser rejeitado;
- como configurar regra raiz;
- como configurar regra descendente;
- como evitar regras órfãs, ciclos e escopos incompatíveis;
- como o `type` identifica etapas como avaliação final, exame, recurso ou recuperação;
- como `regra_dependencia_id` ou relacionamento equivalente conecta a cadeia;
- como a regra raiz continua única por escopo aplicável;
- quais campos são obrigatórios por nível;
- quais campos são incompatíveis por nível;
- quais cenários devem gerar erro de validação.

Inclua exemplos funcionais em texto ou tabelas, sem precisar copiar todos os exemplos da API, para demonstrar configurações típicas:

- regra raiz do fundamental;
- regra de recuperação/exame do fundamental;
- regra raiz do médio com `materias_chave`;
- regra descendente do médio aplicável apenas a algumas matérias;
- regra raiz do superior com fórmula sem período explícito;
- regra superior com limite de pendências.

### 3. Fórmulas por nível

Documente como a fórmula funciona por nível:

- fundamental e médio usam referência com categoria e período explícito, conforme contrato existente;
- superior usa referência sem período explícito no payload da regra;
- no superior, o backend infere o período/semestre a partir da matéria/escopo avaliado;
- o parser próprio valida a fórmula declarativa;
- categorias envolvidas são extraídas da fórmula;
- a documentação não deve sugerir uso de `eval`, scripts ou código dinâmico;
- erros de fórmula devem ser explicados como erros de validação, não como falhas operacionais inesperadas.

Explique cenários como:

- fórmula válida para fundamental/médio;
- fórmula inválida para fundamental/médio por ausência de período;
- fórmula válida para superior sem período;
- fórmula inválida para superior com período explícito;
- ausência de notas necessárias para calcular determinada matéria;
- diferença entre fórmula configurada e notas carregadas para cada matéria.

### 4. Avaliação final no ensino fundamental

Documente o fluxo do fundamental, incluindo:

- escopo por `ano_escolar_fundamental` do estudante;
- uso de `anos_academicos` apenas em regra fundamental;
- avaliação de cada matéria disciplinar aplicável ao ano do estudante;
- cálculo individual de `nota_final` por matéria;
- aprovação direta apenas se todas as matérias exigidas atingirem a nota mínima;
- reprovação/recuperação quando uma ou mais matérias ficarem abaixo da mínima;
- acionamento de regras descendentes por matéria reprovada;
- impossibilidade de aprovação com pendência no fundamental;
- progressão para próximo ano quando aprovado;
- conclusão do fundamental no último ano;
- comportamento quando a academia não oferta o próximo ano, se esse processo já estiver documentado ou implementado.

### 5. Avaliação final no ensino médio

Documente o fluxo do médio, incluindo:

- escopo por `ano_escolar_medio` do estudante;
- vínculo com curso e matérias disciplinares do curso;
- cálculo de `nota_final` por matéria do curso/ano aplicável;
- obrigatoriedade de `materias_chave` na regra raiz;
- decisão de aprovação direta baseada nas matérias em `materias_chave`, conforme implementação atual;
- validação de que `materias_chave` pertence ao curso/ano/escopo aplicável;
- acionamento de regras descendentes por matéria reprovada;
- criação de pendências apenas depois de esgotar a cadeia aplicável;
- uso de `limite_materias_pendentes`;
- efeito de `pendencia_permitida` por matéria;
- aprovação com pendência;
- reprovação total quando o limite é ultrapassado ou quando alguma matéria não permite pendência;
- bloqueio de progressão/conclusão por `pendencia_nivel_conclusao`;
- retomada do fluxo após regularização das pendências.

Inclua cenários explícitos, por exemplo:

- todas as matérias-chave aprovadas;
- matéria não-chave reprovada, se o sistema suportar diferença prática entre matéria-chave e não-chave;
- matéria-chave reprovada e aprovada em regra descendente;
- matéria reprovada sem regra descendente;
- uma pendência dentro do limite;
- número de pendências acima do limite;
- matéria reprovada com `pendencia_permitida=false`;
- pendência bloqueante no ano de conclusão;
- pendência de curso anterior que não bloqueia o curso atual.

### 6. Avaliação final no ensino superior

Documente o fluxo do superior, incluindo:

- escopo por curso e período/semestre atual;
- uso do `periodo` da matéria e do `semestre_atual` do estudante;
- fórmula sem período explícito;
- inferência automática do período no cálculo;
- cálculo por matéria do curso/período aplicável;
- aprovação direta quando todas as matérias avaliadas atingem a nota mínima;
- acionamento de regras descendentes por matéria reprovada;
- aprovação com pendência quando permitido;
- reprovação total por limite excedido ou matéria sem pendência permitida;
- bloqueio por `pendencia_nivel_conclusao` quando a pendência pertence ao semestre atual/conclusivo;
- retomada automática de progressão ou conclusão após baixa das pendências.

Inclua cenários como:

- estudante aprovado em todas as matérias do semestre;
- estudante enviado para exame em uma matéria;
- estudante aprovado com pendência dentro do limite;
- estudante reprovado por exceder limite;
- estudante reprovado por pendência não permitida;
- estudante com pendência de curso anterior que permanece histórica e não bloqueia o curso atual.

### 7. Regras descendentes

Documente, com profundidade, a cadeia de regras:

- o que é uma regra descendente;
- quando ela é executada;
- como se relaciona com a regra ascendente;
- como `materias_aplicaveis` restringe a execução;
- por que a descendente só recalcula matérias abaixo da mínima na etapa anterior;
- como uma matéria fora da lista de aplicação não é recalculada;
- como a aprovação/reprovação da descendente continua sendo por matéria;
- como a cadeia termina;
- o que acontece quando não há descendente aplicável;
- como o sistema decide entre reprovação total e pendência após a última etapa aplicável.

Inclua cenários de recuperação/exame/recurso com uma ou mais matérias.

### 8. Resultados por matéria, eventos, projeções e auditoria

Documente que o resultado da avaliação final deve ser rastreável por matéria.

Explique:

- que cada item avaliado registra `materia_id` e `nota_final`;
- quando aplicável, registra aprovado/reprovado por matéria;
- qual regra foi usada em cada etapa;
- que snapshots preservam fórmula, notas e configurações relevantes;
- que a decisão geral não deve ser explicada como média global única;
- que avaliações já registradas são eventos auditáveis;
- que alterações posteriores de regra ou matéria não devem reescrever silenciosamente decisões passadas;
- o papel das projeções para consulta.

### 9. Pendências de matérias

Documente o novo recurso persistente de matérias pendentes.

Explique:

- que pendências só existem para médio e superior;
- que o fundamental não permite pendência;
- que pendência só é considerada após reprovação na cadeia aplicável;
- que pendências são criadas apenas quando o estudante aprova com pendência;
- que pendências não devem ser criadas quando o estudante reprova totalmente;
- que o limite de pendências é avaliado antes da criação;
- que uma pendência aberta duplicada no mesmo escopo não deve ser criada;
- que o recurso mantém histórico de pendências abertas e baixadas;
- que curso salvo na pendência importa para bloqueios;
- que pendências de curso anterior permanecem históricas, mas não bloqueiam o curso atual;
- que criação e baixa devem ser auditáveis por evento/projeção.

Inclua a lista funcional dos dados que uma pendência precisa carregar, como estudante, matéria, academia, curso, nível, ano letivo, escopo acadêmico, regra/evento de origem, status pendente e timestamps.

### 10. Bloqueio por `pendencia_nivel_conclusao`

Documente os cenários de bloqueio:

- médio com pendência no ano de conclusão;
- superior com pendência no semestre/período de conclusão;
- aprovação com pendência sem progressão automática;
- estudante mantido em estado de regularização;
- baixa de todas as pendências bloqueantes retomando o fluxo normal;
- pendências não bloqueantes permitindo progressão conforme regra implementada;
- pendências de curso anterior ignoradas para bloqueio do curso atual.

### 11. Avaliação/regularização de matérias pendentes

Documente o fluxo de regularização:

- como uma matéria pendente é avaliada posteriormente;
- que essa avaliação deve gerar evento auditável próprio;
- quais dados mínimos devem ser registrados;
- aprovação baixando a pendência;
- reprovação mantendo a pendência aberta;
- retomada de progressão/conclusão quando não restam pendências bloqueantes ou abertas relevantes;
- diferença entre regularização de pendência e avaliação final normal.

### 12. Cenários de erro e validação

Inclua seção específica com cenários que devem falhar, por exemplo:

- payload com `tipo_ensino`;
- regra sem `nivel` quando a academia exige envio explícito;
- academia mista tentando criar regra superior;
- academia não mista tentando criar regra de nível incompatível;
- `anos_academicos` em regra média ou superior;
- regra fundamental sem `anos_academicos` quando exigido;
- regra média raiz sem `materias_chave`;
- `materias_chave` fora do curso/ano aplicável;
- descendente com matérias fora do escopo ascendente;
- descendente órfã ou cíclica;
- fórmula superior com período explícito;
- fórmula fundamental/média sem período explícito;
- `limite_materias_pendentes` ausente em médio/superior;
- `limite_materias_pendentes` negativo;
- `limite_materias_pendentes` em fundamental;
- pendência em matéria fundamental;
- tentativa de criar duplicidade de pendência aberta;
- tentativa de progredir/concluir com pendência bloqueante do curso atual.

### 13. Relação com outros processos de negócio

Como adicional, após concluir a documentação principal de avaliação final, revise os demais processos de negócio em `docs/Spuri - Documentação.md` e atualize apenas o que for necessário para manter coerência.

Essa parte é secundária e deve ser feita apenas depois da prioridade principal.

Considere atualizar, se necessário:

- matrícula e vínculo acadêmico;
- lançamento de notas;
- progressão de ano no fundamental e médio;
- progressão de semestre no superior;
- conclusão/finalização de ciclo;
- transferência ou mudança de curso;
- status do estudante;
- histórico acadêmico;
- rebuild/projeções;
- permissões e responsabilidades operacionais relacionadas.

Se perceber que algum processo de negócio importante não existe e precisa ser criado para explicar corretamente avaliação final, pendências ou regularização, adicione uma nova subseção. Faça isso apenas quando realmente melhorar a compreensão do modelo atual.

## Regras de qualidade da documentação

- Escreva em português claro, técnico e funcional.
- Evite linguagem ambígua como "talvez", "provavelmente" ou "deve futuramente" para comportamento já implementado.
- Quando algo for apenas uma preparação estrutural ou uma limitação atual, deixe isso explícito.
- Não prometa comportamento que o código ainda não suporta.
- Não mantenha contradições entre seções antigas e novas.
- Não duplique grandes blocos de texto sem necessidade.
- Use tabelas quando ajudarem a comparar níveis, campos obrigatórios, cenários e decisões.
- Use exemplos curtos e funcionais.
- Diferencie claramente regra de configuração, execução automática, resultado registrado e pendência.
- Mantenha a documentação de API como referência de contrato HTTP, mas deixe `docs/Spuri - Documentação.md` como a fonte funcional do processo de negócio.

## Critérios de aceite

- `docs/Spuri - Documentação.md` descreve de forma aprofundada o modelo atual de avaliação final por matéria na seção **"5. Processos de Negócio"**.
- A documentação explica como montar/criar regras de avaliação final por nível.
- A documentação usa `nivel` como nomenclatura pública das regras e não reintroduz `tipo_ensino` como contrato aceito.
- A documentação explica diferenças entre fundamental, médio e superior.
- A documentação explica fórmulas por nível, incluindo período inferido no superior.
- A documentação explica regras descendentes por matéria.
- A documentação explica `materias_chave`, `materias_aplicaveis` e `limite_materias_pendentes`.
- A documentação explica pendências de matérias, bloqueios e regularização.
- A documentação lista cenários de sucesso, reprovação, recuperação, aprovação com pendência e erro de validação.
- Outros processos de negócio foram ajustados somente quando necessário para não contradizer avaliação final e pendências.
- Não há trechos conflitantes na documentação funcional sobre avaliação final antiga como nota global única.
- A documentação final é consistente com o comportamento implementado e com `docs/Spuri - API.md`.

## Verificações obrigatórias

Execute ao menos:

```bash
rg -n "tipo_ensino|tipoEnsino|tipo ensino|avaliação final|avaliacao final|pendência|pendencia|materias_chave|materias_aplicaveis|limite_materias_pendentes|pendencia_nivel_conclusao" docs tarefas internal migrations
```

Use o resultado para confirmar que a documentação nova não ficou contraditória e que referências a `tipo_ensino` sejam apenas históricas, explicativas ou de remoção, nunca instruções de uso do contrato novo.

Se alterar apenas documentação, não é obrigatório executar a suíte de testes Go. Ainda assim, rode uma verificação textual suficiente para revisar os termos e as seções alteradas.
