#!/usr/bin/env python3
"""Create evidence-anchored gold AFTER source annotations are frozen."""
import json,pathlib,argparse,hashlib
root=pathlib.Path(__file__).resolve().parent
p=argparse.ArgumentParser();p.add_argument('--build',type=pathlib.Path,required=True);a=p.parse_args()
cards={c['situation_id']:c for c in json.loads((a.build/'cards.json').read_text())}
old=json.loads((root.parent/'dino-crisis-fts/questions.json').read_text())
mapping={'beginning':'opening-gail','first-generator':'generator-first','panel-code':'panel-keys','office-safe':'safe-office-early','gas':'gas-neutralize','hemostat':'item-hemostat','rick-branch':'first-rick-immediate','gail-branch':'first-gail-immediate','b1-chip':'b1-chip-panel','b2-chips':'b2-chips-panel','office-id':'office-id-early','handgun-slides':'handgun-slides','shotgun-location':'shotgun-first','mix-menu':'menu-mix','turn-around':'controls-turn','med-pak-l':'item-medpak-l','emergency-box':'emergency-green','planning-disc-safe':'third-rick-safe','second-generator':'generator-second'}
# Exact canonical source spans are the gold identity, independent of chunks.
cases=[]
for c in old:
 c=dict(c);c.pop('relevant_sequences',None);c['evidence']=[{'source_span':s} for s in cards[mapping[c['id']]]['source_spans']] if c['id'] in mapping else [];c['split']='existing-regression';cases.append(c)
# Independent source cases, selected by failure category rather than old chunks.
new=[
 ('fresh-green-items','Какие два предмета лежат за зелёной дверью в самом начале, когда Regina только получила управление?','Resuscitation и BG Area Key. Не BG Room B1 Key.','opening-gail'),
 ('return-small-key','Я вернулся в infirmary, где раньше была ID Card. Что в левом и правом ящике под Small Size Key? Можно открыть оба?','Левый: Resuscitation; правый: Multiplier. Нужно выбрать один.','small-size-boxes'),
 ('red-box','У красного ящика в комнате лифтов: сколько Plug нужно и что хранится внутри?','Два Plug; боеприпасы.','emergency-red'),
 ('gail-pipes','Первая ветка Gail. В каком порядке соединять цветные газовые трубы, чтобы запустить большой лифт?','5=RED, 3=GREEN, 1=BLUE, 2=RED, 4=GREEN, 6=BLUE.','first-gail-pipes'),
 ('rick-pipes','Первая ветка Rick, газовые трубы перед большим лифтом. Назови все шесть операций по порядку.','5=RED, 3=GREEN, 1=BLUE, 2=RED, 4=GREEN, 6=BLUE.','first-rick-pipes'),
 ('second-rick-memory','Во втором выборе я следую идее Рика. Куда идти, чтобы открыть люк, и что будет после двух ошибок?','Simulator room, панель в конце, игра памяти; после двух ошибок система отключается, reset и повторить.','second-rick'),
 ('second-gail-route','Второй выбор: иду за Gail после запертых дверей. Куда возвращаться?','Комната лифта, затем контейнеры через ранее закрытые ворота.','second-gail'),
 ('battery-acd','После прибытия большого лифта пропал свет. Я вернул батарею в исходное устройство. Какие пункты выбрать?','A, C, D, Cancel.','restore-power-acd'),
 ('disc-yellow-leading-zero','Третья стратегия Рика: Planning Disc у панели рядом с жёлтой папкой в комнате C. Какой цифровой код?','0367 с ведущим нулём.','third-rick-0367'),
 ('disc-blue-leading-zero','Третья стратегия Rick: Planning Disc у панели с синим кругом. Какой код для двух недостающих деталей?','0204 с ведущим нулём.','third-rick-0204'),
 ('late-gail-id','Я выбрал третью стратегию Gail, получил Initializer и Stabilizer и использую ID Card у тела в комнате первого B2 чипа. Какой пароль и выбор?','31415, Yes.','third-gail-id'),
 ('late-rick-id','Третья стратегия Rick: я собрал Initializer и Stabilizer и обновляю ID Card на компьютере у тела. Какой пароль и ответ?','31415, Yes.','third-rick-id'),
 ('b2-ru-plain','На экране у стены стоят два ключевых чипа подвала B2. Какие четыре цифры вводятся до головоломки с блоками?','0392.','b2-chips-panel'),
 ('english-first-generator','At the first generator after Gail stands guard, what is the button sequence before pulling the four levers?','CENTER, RIGHT, CENTER, CANCEL.','generator-first'),
 ('unanswerable-battery-code','Какой числовой код из шести цифр нужен для Startup Battery R?','Числовой код не подтверждён: источник даёт последовательность кнопок. Уточнить устройство; не выдумывать код.',None),
 ('ambiguous-computer','Какой пароль компьютера?','Уточнить комнату и этап: в прохождении несколько разных компьютеров и паролей.',None),
 ('unanswerable-easy','На Easy за зелёной дверью в начале лежат те же предметы? Назови точный список для Easy.','Точный список для Easy неизвестен; источник написан для Normal и предупреждает об отличии предметов.',None),
]
for id,q,expected,card in new:
 cases.append({'id':id,'question':q,'expected':expected,'evidence':[] if card is None else [{'source_span':s} for s in cards[card]['source_spans']],'split':'independent-source-diagnostic'})
# Both facts are required, so a single high cosine/correct code cannot count
# as complete evidence for a compound question.
cases.append({'id':'b1-b2-distinction','question':'Напомни отдельно цифровые пароли: для одного B1 Key Chip на синем экране и для двух B2 чипов перед блоками.','expected':'B1: 3695. B2: 0392. Не смешивать.','evidence':[{'source_span':s} for k in ['b1-chip-panel','b2-chips-panel'] for s in cards[k]['source_spans']],'split':'independent-source-diagnostic'})
path=a.build/'questions.json'
if path.exists():raise SystemExit('case file exists; choose fresh build/version')
path.write_text(json.dumps(cases,ensure_ascii=False,indent=2)+'\n');print(len(cases),'cases;',sum(bool(c['evidence']) for c in cases),'answerable')
