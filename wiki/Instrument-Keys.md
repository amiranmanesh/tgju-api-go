# Instrument keys

Every row has a `key` — tgju's own identifier for the instrument. It survives page
redesigns and is what you should store; titles are display text and change.

Get the current list from a running instance rather than from this page, which is a
snapshot of a moving target:

```bash
tgju get currency --format csv | cut -d, -f3,4
curl -s localhost:8080/v1/markets/gold/items | jq -r '.items[] | "\(.key)\t\(.title)"'
```

## Currency — `/currency`

| Key | Title |
| --- | --- |
| `price_dollar_rl` | دلار |
| `price_eur` | یورو |
| `price_gbp` | پوند انگلیس |
| `price_aed` | درهم امارات |
| `price_try` | لیر ترکیه |
| `price_chf` | فرانک سوئیس |
| `price_cny` | یوان چین |
| `price_jpy` | ین ژاپن (100 ین) |
| `price_cad` | دلار کانادا |
| `price_aud` | دلار استرالیا |
| `price_nzd` | دلار نیوزیلند |
| `price_sgd` | دلار سنگاپور |
| `price_inr` | روپیه هند |
| `price_pkr` | روپیه پاکستان |
| `price_iqd` | دینار عراق |
| `price_kwd` | دینار کویت |
| `price_syp` | پوند سوریه |
| `price_afn` | افغانی |
| `price_rub` | روبل روسیه |
| `price_dkk`, `price_sek`, `price_nok` | کرون دانمارک، سوئد، نروژ |

Roughly three dozen pairs in total, across two tables that share the caption `عنوان`.

## Gold and silver — `/gold-chart`

**قیمت طلا**

| Key | Title |
| --- | --- |
| `geram18` | طلای 18 عیار / 750 |
| `gold_740k` | طلای 18 عیار / 740 |
| `geram24` | طلای 24 عیار |
| `gold_mini_size` | طلای دست دوم |

**مظنه / مثقال طلا**

| Key | Title |
| --- | --- |
| `mesghal` | مثقال طلا |
| `gold_17` | مثقال / بدون حباب |
| `gold_17_transfer` | حباب آبشده |
| `gold_17_coin` | مثقال / بر مبنای سکه |

**قیمت آبشده**

| Key | Title |
| --- | --- |
| `gold_futures` | آبشده نقدی |
| `gold_melted_transfer` | آبشده معاملاتی |
| `gold_melted_wholesale` | آبشده بنکداری |
| `gold_world_futures` | آبشده کمتر از کیلو |

**قیمت نقره**

| Key | Title |
| --- | --- |
| `silver_925` | گرم نقره 925 |
| `silver_999` | گرم نقره 999 |

**طلا در بورس** — exchange traded gold funds, `ime_fund_*` and similar. Around thirty
rows, and the one table whose timestamps are Persian dates rather than clocks, because the
exchange is closed outside trading hours.

## Coins — `/coin`

**قیمت نقدی** — the spot market

| Key | Title |
| --- | --- |
| `sekee` | سکه امامی |
| `sekeb` | سکه بهار آزادی |
| `nim` | نیم سکه |
| `rob` | ربع سکه |
| `gerami` | سکه گرمی |

**قیمت تک فروشی** — retail, the same five with a `retail_` prefix: `retail_sekee`,
`retail_sekeb`, `retail_nim`, `retail_rob`, `retail_gerami`.

**حباب سکه** — the premium over melt value: `coin_blubber` (امامی), `sekeb_blubber`,
`nim_blubber`, `rob_blubber`, `gerami_blubber`.

**سکه در بورس** — bank-issued coins on the exchange: `gc14` … `gc19`.

**سایر سکه‌ها** — pre-1386 coins: `sekee_down`, `nim_down`, `rob_down`.

## Notes

- The same instrument can appear on two boards under different keys — the spot coin
  (`sekee`) and its retail price (`retail_sekee`) are different rows.
- `/v1/items/{key}` searches every board and takes the first match, in alphabetical order
  of market name: coin, currency, gold. Name the market explicitly when a key might be
  ambiguous: `client.Item(ctx, key, tgju.Gold)`.
- A key that stops appearing means tgju delisted the instrument. The lookup returns
  `ErrNotFound`, not `ErrParse` — nothing is broken, the row is simply gone.
