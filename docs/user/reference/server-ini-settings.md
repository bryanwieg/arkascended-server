# Server INI Settings

Every setting the ARK: Survival Ascended dedicated server reads from
`GameUserSettings.ini` `[ServerSettings]` and `Game.ini`
`[/Script/ShooterGame.ShooterGameMode]`, with its default and what it does.
It includes settings ARK never writes to the INI files. Applies to server
version 98.9 (Steam build 26749470); settings can appear or disappear with ARK
updates.

How to change a setting: [Edit server INI files](../how-to/edit-ini.md).

## Reading this reference

- Settings are grouped by topic. Each topic has a table for
  `GameUserSettings.ini` (GUS) `[ServerSettings]` and one for `Game.ini`
  `[/Script/ShooterGame.ShooterGameMode]`; the table heading names the file
  and section the keys go in.
- **Conf.** says how well the effect is confirmed:
  - ✔: confirmed; **Notes** describe the details.
  - ✔ × / ✔ ÷: the value multiplies / divides the quantity the name
    describes, so the raise/lower direction is confirmed.
  - ✔ lim: confirmed as a cap, threshold or duration.
  - ◐: partly confirmed; **Notes** say what is known.
  - ○: the switch is used, but its exact effect is not confirmed.
  - ✗: no use found; the setting may have no effect in this build.
  - blank: strings, lists and structs.
- **Type = default**: the value type and the default when the key is absent.
  `—` = default unknown.
- **Written** (GUS only): ARK writes the key to `GameUserSettings.ini` on
  first start. Keys without it are still read; add them yourself.
- **Raising changes**: what goes up when the value is raised; `switch` for
  on/off keys. **Raised / True** and **Lowered / False**: the effect of each
  direction. For rows not marked ✔, this text is based on the key's name.
- **Notes**: special values (for example "0 = no limit"), inverted switches
  and other details.
- "(also GUS)" marks Game.ini keys that are also read from GUS
  `[ServerSettings]`. The GUS value always wins: if the key is missing from
  `GameUserSettings.ini`, its GUS default replaces the Game.ini value. Set
  these keys in `GameUserSettings.ini`.
- `RCONEnabled`, `RCONPort`, `ServerAdminPassword` and `ServerPassword` are
  rewritten from the environment on every start
  ([managed keys](../how-to/edit-ini.md#keys-managed-from-the-environment)).
- Servers flagged as official ignore several of these settings: they force
  `ItemStackSizeMultiplier` 1, `MaxTamedDinos` 5000, `TheMaxStructuresInRange`
  10500, and their own tame-limit and buried-treasure timers.

## Reading the multipliers

Most multipliers are "higher = more or faster". These are inverted or easy
to misread:

| Key | Higher value means | For easier/faster |
|---|---|---|
| `PlayerResistanceMultiplier`, `DinoResistanceMultiplier`, `StructureResistanceMultiplier` | takes **more** damage (multiplies incoming damage) | below 1.0 |
| `MatingIntervalMultiplier`, `LayEggIntervalMultiplier`, `PoopIntervalMultiplier` | longer wait between events | lower |
| `BabyCuddleIntervalMultiplier` | longer gap between imprint cuddles | lower, in step with `BabyMatureSpeedMultiplier` |
| `ResourcesRespawnPeriodMultiplier` | resources respawn slower | lower |
| `PassiveTameIntervalMultiplier` | longer between passive-tame feedings | lower |
| `*DrainMultiplier`, `BabyFoodConsumptionSpeedMultiplier`, `CropDecaySpeedMultiplier` | drains or decays faster | lower |
| `FuelConsumptionIntervalMultiplier`, `GlobalSpoilingTimeMultiplier`, `*DecompositionTimeMultiplier`, `*DecayPeriodMultiplier` | things last **longer** | higher |
| `HarvestHealthMultiplier` | nodes have more HP (more hits, more total yield) | higher |

`PerLevelStatsMultiplier_*[N]` (Game.ini): a value of 0 is treated as 1.0.
Several other multipliers are used
only when greater than 0 and otherwise fall back to the default (see their
**Notes**); a 0 default does not necessarily mean "off".
The stat index `N` is 0 health, 1 stamina, 2 torpidity, 3 oxygen, 4 food,
5 water, 6 temperature, 7 weight, 8 melee damage, 9 movement speed,
10 fortitude, 11 crafting speed.


## Settings by topic

246 `GameUserSettings.ini` `[ServerSettings]` keys (45 written by ARK) and
268 `Game.ini` `[/Script/ShooterGame.ShooterGameMode]` keys, grouped by what they affect.
Each topic lists its `GameUserSettings.ini` keys first, then its `Game.ini` keys.
Game.ini array keys take one line per element (`Key[N]=value`); `list` keys
take one `Key=(...)` line per entry.

- [Server, mods & URLs](#server-mods--urls) (20 + 1)
- [Cryopods & companions](#cryopods--companions) (17 + 11)
- [Breeding & imprinting](#breeding--imprinting) (6 + 17)
- [Taming & tame limits](#taming--tame-limits) (11 + 9)
- [XP & levels](#xp--levels) (3 + 33)
- [PvP / PvE rules](#pvp--pve-rules) (20 + 18)
- [Decay & auto-destroy](#decay--auto-destroy) (7 + 4)
- [Structures & building](#structures--building) (40 + 46)
- [Harvesting & resources](#harvesting--resources) (7 + 10)
- [Items, crafting & loot](#items-crafting--loot) (26 + 39)
- [Creatures, spawns & stats](#creatures-spawns--stats) (27 + 33)
- [Transfers & cluster](#transfers--cluster) (11 + 2)
- [Tribes & players](#tribes--players) (10 + 11)
- [Chat, UI & admin](#chat-ui--admin) (27 + 13)
- [World, time & events](#world-time--events) (8 + 6)
- [Missions, outposts & hexagons](#missions-outposts--hexagons) (6 + 15)

### Server, mods & URLs

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `ActiveEvent` |  | string = (empty) |  |  |  |  |  |
| `ActiveMapMod` |  | string = (empty) |  |  |  |  |  |
| `ActiveMods` |  | string = (empty) |  |  |  |  |  |
| `ActiveTotalConversion` |  | string = (empty) |  |  |  |  |  |
| `AdminListURL` |  | string = (empty) |  |  |  |  |  |
| `AutoRestartIntervalSeconds` | ✗ | float = 0 |  | ↑ auto restart interval seconds | Longer | Shorter |  |
| `BadWordListURL` |  | string = (empty) |  |  |  |  |  |
| `BadWordWhiteListURL` |  | string = (empty) |  |  |  |  |  |
| `BanListURL` |  | string = (empty) |  |  |  |  |  |
| `BGOverridePath` |  | string = (empty) |  |  |  |  |  |
| `ClassLiveTunableConfigUrl` |  | string = (empty) |  |  |  |  |  |
| `CosmeticWhitelistOverride` |  | string = (empty) |  |  |  |  |  |
| `CustomDynamicConfigUrl` |  | string = (empty) |  |  |  |  |  |
| `CustomDynamicUndermeshConfigUrl` |  | string = (empty) |  |  |  |  |  |
| `CustomLiveTuningUrl` |  | string = (empty) |  |  |  |  |  |
| `NetModBanlistOverride` |  | string = (empty) |  |  |  |  |  |
| `ServerAdminPassword` |  | string = (empty) |  |  |  |  |  |
| `ServerPassword` |  | string = (empty) |  |  |  |  |  |
| `SpectatorPassword` |  | string = (empty) |  |  |  |  |  |
| `WorldModifier` |  | string = (empty) |  |  |  |  |  |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `LiveTuningFileName` |  | string = — |  |  |  |  |

### Cryopods & companions

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `AllowCryoFridgeOnSaddle` | ✔ | bool = False |  | switch | Cryo fridge on saddle allowed | Cryo fridge on saddle not allowed | When off, Cryofridges can't be placed on saddles. |
| `ArmadoggoDeathCooldown` | ✔ | float = 0 |  | ↑ armadoggo death cooldown | Longer | Shorter | If > 0, the companion's post-death cryo duration (seconds); 0 = creature default. |
| `CompanionsDeathCooldown` | ✔ | float = 0 |  | ↑ companions death cooldown | Longer | Shorter | If > 0, the companion's post-death cryo duration (seconds); 0 = creature default. |
| `CryoHospitalHoursToDrainTorpor` | ✔ | float = 1 | yes | ↑ drain rate | Drains faster | Drains slower | In the Cryo Hospital, torpor drain rate = full amount / this hours; higher = slower. |
| `CryoHospitalHoursToRegenFood` | ✔ | float = 24 | yes | ↑ cryo hospital hours to regen food | Longer | Shorter | In the Cryo Hospital, food regen rate = full amount / this hours; higher = slower. |
| `CryoHospitalHoursToRegenHP` | ✔ | float = 1 | yes | ↑ cryo hospital hours to regen HP | Longer | Shorter | In the Cryo Hospital, health regen rate = full amount / this hours; higher = slower. |
| `CryoHospitalMatingCooldownReduction` | ✔ | float = 2 | yes | ↑ mating cooldown reduction | Mating cooldown shrinks faster in the Cryo Hospital | Slower | On release, the mating cooldown is reduced by time spent in the Cryo Hospital × this. |
| `CryopodFridgeCooldownTime` | ✔ | float = 90 |  | ↑ cryopod fridge cooldown time | Longer | Shorter | Seconds before a cryopod can be taken out of a Cryofridge again. |
| `CryopodNerfDamageMult` | ✔ | float = 0 |  | ↑ damage dealt after uncryo | Freshly released creatures deal more damage (milder nerf) | Deal less damage (harsher nerf) | Damage multiplier during the cryopod nerf. |
| `CryopodNerfDuration` | ✔ | float = 0 |  | ↑ nerf duration | The post-cryo nerf lasts longer | Shorter | Duration of the cryopod nerf (seconds). |
| `CryopodNerfIncomingDamageMult` | ✗ | float = 0 |  | ↑ damage taken after uncryo | Freshly released creatures take more damage | Take less damage |  |
| `CryopodNerfIncomingDamageMultPercent` | ✔ | float = 0 |  | ↑ damage taken after uncryo | Freshly released creatures take more damage | Take less damage | Incoming-damage multiplier during the cryopod nerf. |
| `DisableCryopodEnemyCheck` | ✔ | bool = False |  | switch | Cryopod enemy check off | Cryopod enemy check on | When on, the 'enemies nearby' check is skipped. |
| `DisableCryopodFridgeRequirement` | ✔ | bool = False |  | switch | Cryopod fridge requirement off | Cryopod fridge requirement on | When on, cryopods don't need a Cryofridge/powered storage. |
| `EnableCryopodNerf` | ✔ | bool = False |  | switch | Cryopod nerf on | Cryopod nerf off | When on, released creatures get the cryopod nerf (CryopodNerfDuration, CryopodNerfDamageMult, CryopodNerfIncomingDamageMultPercent). |
| `EnableCryoSicknessPVE` | ✔ | bool = False |  | switch | Cryo sickness PvE on | Cryo sickness PvE off | When on, cryo sickness applies on PvE too. |
| `YoungIceFoxDeathCooldown` | ✔ | float = 0 |  | ↑ young ice fox death cooldown | Longer | Shorter | If > 0, post-death cryo duration (seconds); 0 = creature default. |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `bAllowCryoFridgeOnSaddle` (also GUS) | ✔ | bool = False | switch | Cryo fridge on saddle allowed | Cryo fridge on saddle not allowed | When off, Cryofridges can't be placed on saddles. |
| `ArmadoggoDeathCooldown` (also GUS) | ✔ | float = 0 | ↑ armadoggo death cooldown | Longer | Shorter | If > 0, the companion's post-death cryo duration (seconds); 0 = creature default. |
| `CompanionsDeathCooldown` (also GUS) | ✔ | float = 0 | ↑ companions death cooldown | Longer | Shorter | If > 0, the companion's post-death cryo duration (seconds); 0 = creature default. |
| `CryoHospitalHoursToDrainTorpor` (also GUS) | ✔ | float = 1 | ↑ drain rate | Drains faster | Drains slower | In the Cryo Hospital, torpor drain rate = full amount / this hours; higher = slower. |
| `CryoHospitalHoursToRegenFood` (also GUS) | ✔ | float = 24 | ↑ cryo hospital hours to regen food | Longer | Shorter | In the Cryo Hospital, food regen rate = full amount / this hours; higher = slower. |
| `CryoHospitalHoursToRegenHP` (also GUS) | ✔ | float = 1 | ↑ cryo hospital hours to regen HP | Longer | Shorter | In the Cryo Hospital, health regen rate = full amount / this hours; higher = slower. |
| `CryoHospitalMatingCooldownReduction` (also GUS) | ✔ | float = 2 | ↑ mating cooldown reduction | Mating cooldown shrinks faster in the Cryo Hospital | Slower | On release, the mating cooldown is reduced by time spent in the Cryo Hospital × this. |
| `CryopodFridgeCooldownTime` (also GUS) | ✔ | float = 90 | ↑ cryopod fridge cooldown time | Longer | Shorter | Seconds before a cryopod can be taken out of a Cryofridge again. |
| `bDisableCryopodEnemyCheck` (also GUS) | ✔ | bool = False | switch | Cryopod enemy check off | Cryopod enemy check on | When on, the 'enemies nearby' check is skipped. |
| `bDisableCryopodFridgeRequirement` (also GUS) | ✔ | bool = False | switch | Cryopod fridge requirement off | Cryopod fridge requirement on | When on, cryopods don't need a Cryofridge/powered storage. |
| `YoungIceFoxDeathCooldown` (also GUS) | ✔ | float = 0 | ↑ young ice fox death cooldown | Longer | Shorter | If > 0, post-death cryo duration (seconds); 0 = creature default. |

### Breeding & imprinting

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `AllowAnyoneBabyImprintCuddle` | ✔ | bool = False |  | switch | Anyone baby imprint cuddle allowed | Anyone baby imprint cuddle not allowed | When on, anyone may do imprint cuddles, not just the imprinter. |
| `DisableGeneTraits` | ✔ | bool = False |  | switch | Gene traits off | Gene traits on | When on, wild creatures spawn without gene traits. Can also be toggled at runtime by an admin command. |
| `DisableImprintDinoBuff` | ✔ | bool = False |  | switch | Imprint dino buff off | Imprint dino buff on | When on, the imprint damage bonus is not applied. |
| `FreezeReaperPregnancy` | ✗ | bool = False |  | switch | On: freeze reaper pregnancy | Off |  |
| `PreventMateBoost` | ✔ | bool = False |  | switch | Mate boost blocked | Mate boost allowed | When on, the mate-boost stack timer is skipped: no mate boost. |
| `PreventTekChemBenchBabies` | ✔ | bool = False |  | switch | Tek chem bench babies blocked | Tek chem bench babies allowed | Tek chemistry bench: when on, its 'babyfy creature' option is refused. |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `BabyCuddleGracePeriodMultiplier` | ✔ × | float = 1 | ↑ grace period | More time to cuddle before imprint is lost | Less time to cuddle |  |
| `BabyCuddleIntervalMultiplier` | ✔ | float = 1 | ↑ time between cuddles | Longer gap between imprint cuddles | Cuddles come more often (lower with BabyMatureSpeedMultiplier) | Cuddle interval = random(min, max) of the species interval, each multiplied by this. |
| `BabyCuddleLoseImprintQualitySpeedMultiplier` | ✔ × | float = 1 | ↑ imprint loss speed | Imprint is lost faster after a missed cuddle | Imprint is lost slower |  |
| `BabyFoodConsumptionSpeedMultiplier` | ✔ | float = 1 | ↑ baby food use | Babies eat faster | Babies eat slower | Baby food drain multiplied by this. |
| `BabyImprintAmountMultiplier` | ✗ | float = 1 | ↑ imprint per cuddle | More imprint % per cuddle | Less imprint % per cuddle |  |
| `BabyImprintingStatScaleMultiplier` | ✔ × | float = 1 | ↑ imprint stat bonus | Bigger stat bonus from imprinting | Smaller imprint bonus |  |
| `BabyMatureSpeedMultiplier` | ✔ | float = 1 | ↑ maturation speed | Babies grow up faster | Babies grow up slower | Maturation += dt x rate x this (capped at 1.0). |
| `EggHatchSpeedMultiplier` | ✔ | float = 1 | ↑ hatch speed | Eggs hatch faster | Eggs hatch slower | Incubation += dt x rate x this. |
| `ImprintLimit` | ✔ lim | float = -1 | ↑ imprint limit | Higher limit | Lower limit |  |
| `LayEggIntervalMultiplier` | ✔ × | float = 1 | ↑ time between eggs | Eggs laid less often | Eggs laid more often |  |
| `MatingIntervalMultiplier` | ✔ | float = 1 | ↑ wait between matings | Longer wait between matings | Mating available sooner | Next mating time = now + interval x this. |
| `MatingSpeedMultiplier` | ✔ × | float = 1 | ↑ speed | Faster | Slower |  |
| `MutagenLevelBoost` | ✔ | int[12] = [0]=5, [1]=5, [7]=5, [8]=5; others 0 | ↑ levels per stat from Mutagen | More levels added per stat | Fewer | Levels added per stat (index) by Mutagen on non-bred creatures. |
| `MutagenLevelBoost_Bred` | ✔ | int[12] = [0]=1, [1]=1, [7]=1, [8]=1; others 0 | ↑ levels per stat from Mutagen (bred) | More levels added per stat | Fewer | Levels added per stat (index) by Mutagen on bred creatures. |
| `PoopIntervalMultiplier` | ✔ | float = 1 | ↑ time between poops | Poops less often | Poops more often | Timer += dt / this; the next interval is also multiplied by this. |
| `bPreventTekChemBenchBabies` (also GUS) | ✔ | bool = False | switch | Tek chem bench babies blocked | Tek chem bench babies allowed | Tek chemistry bench: when on, its 'babyfy creature' option is refused. |
| `ServerGeneTraitSpawnRateMultiplier` | ✔ | float = 1 | ↑ server gene trait spawn rate | More / higher | Less / lower | Number of gene traits rolled x this. |

### Taming & tame limits

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `bCheckSoftTameLimitOnTick` | ✔ | bool = False |  | switch | On: check soft tame limit on tick | Off | When on, the soft tame limit is checked continuously instead of only through the deletion countdown. |
| `CheckSoftTameLimitOnTick` | ✔ | bool = False |  | switch | On: check soft tame limit on tick | Off | When on, the soft tame limit is checked continuously instead of only through the deletion countdown. |
| `DestroyTamesOverTheSoftTameLimit` | ✔ | bool = False |  | switch | On: destroy tames over the soft tame limit | Off | When on, the soft tame-limit deletion countdown runs (tames over MaxTamedDinos_SoftTameLimit get destroyed). |
| `bEnableServerDestroyTamesAboveSoftTameLimit` | ○ | bool = False |  | switch | Server destroy tames above soft tame limit on | Server destroy tames above soft tame limit off |  |
| `ForceExploitedTameDeletion` | ✔ | bool = False |  | switch | Exploited tame deletion forced | Not forced | When on, tames flagged as exploited are deleted. |
| `MaxPersonalTamedDinos` | ✔ | int = 250 |  | ↑ personal tame cap | More tames per tribe | Fewer tames per tribe; 0 = no limit | 0 = no limit; otherwise per-tribe personal tame cap. |
| `MaxTamedDinos` | ✔ | float = 5000 | yes | ↑ server tame cap | More tames allowed server-wide | Fewer tames allowed | Server-wide tame cap. |
| `MaxTamedDinos_SoftTameLimit` | ✔ lim | int = 5000 | yes | ↑ max tamed dinos soft tame limit | Higher limit | Lower limit |  |
| `MaxTamedDinos_SoftTameLimit_CountdownForDeletionDuration` | ✔ | int = 604800 | yes | ↑ time before over-limit tames are deleted | Over-limit tames are deleted later | Deleted sooner | Seconds before tames over the soft limit are deleted. |
| `PersonalTamedDinosSaddleStructureCost` | ✔ | int = 0 |  | ↑ tame-limit cost of saddle structures | Saddle structures use more of the personal tame limit | Use less; 0 = free | Structures on a saddle count toward the personal tame limit at this cost. |
| `TamingSpeedMultiplier` | ✔ × | float = 1 |  | ↑ taming speed | Faster taming | Slower taming |  |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `bAllowUnclaimDinos` | ✔ | bool = True | switch | Unclaim dinos allowed | Unclaim dinos not allowed | Off = creatures cannot be unclaimed. |
| `DestroyTamesOverLevelClamp` | ✔ lim | int = 0 | ↑ destroy tames over level clamp | Higher limit | Lower limit |  |
| `bDisableDefaultDinoTaming` | ✔ | bool = False | switch | Default dino taming off | Default dino taming on | When on, normal taming is disabled for every creature class except those in PreventDisableDefaultDinoTameClassNames. |
| `bDisableDinoTaming` | ✔ | bool = False | switch | Dino taming off | Dino taming on | When on, creatures gain no taming progress: no taming. |
| `bForceExploitedTameDeletion` (also GUS) | ✔ | bool = False | switch | Exploited tame deletion forced | Not forced | When on, tames flagged as exploited are deleted. |
| `PassiveTameIntervalMultiplier` | ✔ × | float = 1 | ↑ time between feedings | Passive tames need feeding less often (slower tame) | Feedings come sooner (faster tame) |  |
| `PreventDinoTameClassNames` |  | list = (empty) |  |  |  |  |
| `PreventDisableDefaultDinoTameClassNames` |  | list = (empty) |  |  |  |  |
| `bUseTameLimitForStructuresOnly` | ✔ | bool = False | switch | Use tame limit for structures only | Don't use tame limit for structures only | When on, the personal tame limit (MaxPersonalTamedDinos) no longer counts tames; only saddle structures count, at PersonalTamedDinosSaddleStructureCost each. |

### XP & levels

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `DifficultyOffset` | ✔ | float = 0 |  | ↑ wild levels | Higher wild levels (toward the map maximum) | Lower wild levels; 0 = difficulty 1.0 | Difficulty = lerp(map min, map max difficulty, this); 0 = difficulty 1.0. Ignored when OverrideOfficialDifficulty > 0. |
| `OverrideOfficialDifficulty` | ✔ | float = 0 |  | ↑ wild levels | Higher max wild level (30 × value) | Lower; 0 = use DifficultyOffset | If > 0, replaces the difficulty value outright (max wild level = 30 x difficulty); 0 = use DifficultyOffset. |
| `XPMultiplier` | ✔ × | float = 1 |  | ↑ XP gained | More XP from everything | Less XP |  |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `bAllowFlyerSpeedLeveling` | ✔ | bool = False | switch | Flyer speed leveling allowed | Flyer speed leveling not allowed | A flyer's 'can level speed' flag is set only when this is on. |
| `bAllowSpeedLeveling` | ✔ | bool = False | switch | Speed leveling allowed | Speed leveling not allowed | Guards levelling the movement-speed stat (index 9); off = speed cannot be levelled (players). |
| `bAllowUnlimitedRespecs` | ✔ | bool = False | switch | Unlimited respecs allowed | Unlimited respecs not allowed | When off, each respec records a real-time timestamp (respec cooldown); on = no cooldown. |
| `AlphaKillXPMultiplier` | ✔ | float = 1 | ↑ XP gained | More XP | Less XP | Selected by kill type, multiplied with KillXPMultiplier and base XP. |
| `bAutoUnlockAllEngrams` | ✗ | bool = False | switch | On: auto unlock all engrams | Off |  |
| `BossKillXPMultiplier` | ✔ | float = 1 | ↑ XP gained | More XP | Less XP | Selected by kill type, multiplied with KillXPMultiplier and base XP. |
| `CaveKillXPMultiplier` | ✔ | float = 1 | ↑ XP gained | More XP | Less XP | Selected by kill type, multiplied with KillXPMultiplier and base XP. |
| `CraftXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |  |
| `EngramEntryAutoUnlocks` |  | list = (empty) |  |  |  |  |
| `ExplorerNoteXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |  |
| `GenericXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |  |
| `HarvestXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |  |
| `KillXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |  |
| `LevelExperienceRampOverrides` |  | list = (empty) |  |  |  |  |
| `MaxDifficulty` | ✔ | bool = False | switch | Difficulty forced to 5.0 (levels up to 150) | Normal difficulty rules | When on, the difficulty value is forced to 5.0 (wild levels up to 150). |
| `bOnlyAllowSpecifiedEngrams` | ✗ | bool = False | switch | Only allow specified engrams | Not restricted |  |
| `OverrideEngramEntries` |  | list = (empty) |  |  |  |  |
| `OverrideMaxExperiencePointsDino` | ✗ | int = 0 | ↑ creature XP cap | Tames can earn more total XP | Lower cap; 0 = game default |  |
| `OverrideMaxExperiencePointsPlayer` | ✗ | int = 0 | ↑ player XP cap | Players can earn more total XP (higher max level reachable) | Lower XP cap; 0 = game default |  |
| `OverrideNamedEngramEntries` |  | list = (empty) |  |  |  |  |
| `OverridePlayerLevelEngramPoints` |  | list = (empty) |  |  |  |  |
| `PerLevelStatsMultiplier_DinoTamed` | ✔ | float[12] = [0]=0.2, [8]=0.17; others 0 | ↑ stat gain per level | More stat per level point | Less stat per level point; 0 = 1.0 | Per-stat level-up multiplier; 0 is treated as 1.0. |
| `PerLevelStatsMultiplier_DinoTamed_Add` | ✔ | float[12] = [0]=0.14, [8]=0.14; others 0 | ↑ stat gain per level | More stat per level point | Less stat per level point; 0 = 1.0 | Per-stat level-up multiplier; 0 is treated as 1.0. |
| `PerLevelStatsMultiplier_DinoTamed_Affinity` | ✔ | float[12] = [0]=0.44, [8]=0.44; others 0 | ↑ stat gain per level | More stat per level point | Less stat per level point; 0 = 1.0 | Per-stat level-up multiplier; 0 is treated as 1.0. |
| `PerLevelStatsMultiplier_DinoWild` | ✔ | float[12] = all 0 | ↑ stat gain per level | More stat per level point | Less stat per level point; 0 = 1.0 | Per-stat level-up multiplier; 0 is treated as 1.0. |
| `PerLevelStatsMultiplier_Player` | ✔ | float[12] = all 0 | ↑ stat gain per level | More stat per level point | Less stat per level point; 0 = 1.0 | Per-stat level-up multiplier; 0 is treated as 1.0. |
| `PlayerBaseStatMultipliers` | ✔ | float[12] = all 0 | ↑ base stat | Higher base value for that stat (index = stat); 0 = unchanged | Lower base value | Per stat: if > 0, the player's base stat value is multiplied by this. |
| `SpecialXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |  |
| `TamedKillXPMultiplier` | ✔ | float = 1 | ↑ XP gained | More XP | Less XP | Selected by kill type, multiplied with KillXPMultiplier and base XP. |
| `UnclaimedKillXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |  |
| `bUseDinoLevelUpAnimations` | ✔ | bool = True | switch | Use dino level up animations | Don't use dino level up animations | When on, creatures play the level-up animation. |
| `bUseSingleplayerSettings` | ✔ | bool = False | switch | Single-player balance (tamed level-ups scaled up) | Normal balance | Single-player balance; also scales the tamed per-level defaults (×2.125 health, ×2.35 melee damage, ×3.57 for the _Add values, ×2.27 for the _Affinity values). |
| `WildKillXPMultiplier` | ✔ | float = 1 | ↑ XP gained | More XP | Less XP | Selected by kill type, multiplied with KillXPMultiplier and base XP. |

### PvP / PvE rules

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `AllowCaveBuildingPvE` | ✔ | bool = False |  | switch | PvE: building in caves allowed | PvE: cave building refused | PvE servers: when off, structures can't be placed inside caves. |
| `AllowCaveBuildingPvP` | ✔ | bool = True |  | switch | PvP: building in caves allowed | PvP: cave building refused | PvP servers: when off, structures can't be placed inside caves. |
| `AllowFlyerCarryPvE` | ✔ | bool = False |  | switch | Flyer carry PvE allowed | Flyer carry PvE not allowed | When on, flyers may pick up tribe-owned targets on PvE. |
| `AllowRaidDinoFeeding` | ✔ | bool = False |  | switch | Raid dino feeding allowed | Raid dino feeding not allowed | Titanosaur / its inventory and Sweet Veggie Cake: when on, raid creatures can be fed. |
| `AllowTeslaCoilCaveBuildingPVP` | ✔ | bool = False |  | switch | Tesla coil cave building PvP allowed | Tesla coil cave building PvP not allowed | When off, refused in caves on PvP. |
| `DisableDinoDecayPvE` | ✔ | bool = False |  | switch | PvE: tames never auto-decay | PvE: tames auto-decay | PvE only: when on, creature decay timers are reset, so tames never auto-decay. |
| `DisablePvEGamma` | ✗ | bool = False |  | switch | PvE gamma off | PvE gamma on |  |
| `DisableStructureDecayPvE` | ✔ | bool = False |  | switch | PvE: structures never auto-decay | PvE: structures auto-decay | PvE only: when on, structure decay timers are reset, so structures never auto-decay. |
| `EnablePvPGamma` | ✗ | bool = False |  | switch | PvP gamma on | PvP gamma off |  |
| `IgnorePVPMountedWeaponryRestrictions` | ◐ | bool = False |  | switch | On: ignore PvP mounted weaponry restrictions | Off | On = PvP mounted-weapon restrictions ignored. |
| `PreventOfflinePvP` | ✗ | bool = False |  | switch | Offline PvP blocked | Offline PvP allowed |  |
| `PreventOfflinePvPInterval` | ✗ | float = 0 |  | ↑ prevent offline PvP interval | Longer | Shorter |  |
| `PvEAllowStructuresAtSupplyDrops` | ✗ | bool = False |  | switch | On: PvE allow structures at supply drops | Off |  |
| `PvEDinoDecayPeriodMultiplier` | ✔ | float = 1 | yes | ↑ time before decay | Tames take longer to auto-decay | Tames decay sooner | Creature decay period × this. |
| `PvEStructureDecayPeriodMultiplier` | ✔ | float = 1 |  | ↑ time before decay | Structures take longer to auto-decay | Structures decay sooner | Structure decay period × this. |
| `PvPDinoDecay` | ✔ | bool = False |  | switch | PvP: tames auto-decay | PvP: no tame decay | PvP only: tames auto-decay only when this is on. |
| `PvPStructureDecay` | ✔ | bool = False |  | switch | PvP: structures auto-decay | PvP: no structure decay | PvP only: structures auto-decay only when this is on. |
| `RaidDinoCharacterFoodDrainMultiplier` | ✔ | float = 1 | yes | ↑ drain rate | Drains faster | Drains slower | Food drain of raid creatures x this. |
| `ServerHardcore` | ✔ | bool = False |  | switch | Hardcore: death resets the survivor | Normal respawn | When on, hardcore death handling runs. |
| `ServerPVE` | ✔ | bool = False |  | switch | PvE server | PvP server | When on, damage between different tribes/players is blocked (PvE). |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `bAllowTeslaCoilCaveBuildingPVP` (also GUS) | ✔ | bool = False | switch | Tesla coil cave building PvP allowed | Tesla coil cave building PvP not allowed | When off, refused in caves on PvP. |
| `AutoPvEStartTimeSeconds` | ✔ lim | float = 0 | ↑ auto PvE start time seconds | Longer | Shorter |  |
| `AutoPvEStopTimeSeconds` | ✔ lim | float = 0 | ↑ auto PvE stop time seconds | Longer | Shorter |  |
| `bAutoPvETimer` | ✔ | bool = False | switch | On: auto PvE timer | Off | When on, the server switches between PvE and PvP by time of day (in-game, or real time with bAutoPvEUseSystemTime), using AutoPvEStartTimeSeconds and AutoPvEStopTimeSeconds. |
| `bAutoPvEUseSystemTime` | ✔ | bool = False | switch | On: auto PvE use system time | Off | When on, the auto-PvE window uses the server's real clock; off = in-game time. |
| `bDisableFriendlyFire` | ✔ | bool = False | switch | Friendly fire off | Friendly fire on | When on, friendly fire is blocked. |
| `bDisableRailgunPVP` | ✔ | bool = False | switch | Railgun PvP off | Railgun PvP on | When on, it can't fire on PvP. |
| `DisableRailgunPVP` | ✔ | bool = False | switch | Railgun PvP off | Railgun PvP on | When on, it can't fire on PvP. |
| `bIncreasePvPRespawnInterval` | ✔ | bool = True | switch | On: increase PvP respawn interval | Off | Switches on the PvP respawn penalty that IncreasePvPRespawnInterval* configure. |
| `IncreasePvPRespawnIntervalBaseAmount` | ✔ | float = 60 | ↑ first PvP respawn penalty | Longer first respawn penalty | Shorter | First PvP respawn penalty (seconds); later ones scale by IncreasePvPRespawnIntervalMultiplier, capped at MaxAllowedRespawnInterval. |
| `IncreasePvPRespawnIntervalCheckPeriod` | ✔ lim | float = 300 | ↑ increase PvP respawn interval check period | Longer | Shorter |  |
| `IncreasePvPRespawnIntervalMultiplier` | ✔ × | float = 2 | ↑ penalty growth | Respawn penalty grows faster with repeat deaths | Grows slower |  |
| `MaxAllowedRespawnInterval` | ✔ | float = 1800 | ↑ respawn penalty cap | Longer maximum respawn delay | Shorter | Cap on the PvP respawn delay (seconds). |
| `PreventOfflinePvPConnectionInvincibleInterval` | ✗ | float = 5 | ↑ prevent offline PvP connection invincible interval | Longer | Shorter |  |
| `bPvEAllowTribeWar` | ✔ | bool = True | switch | On: PvE allow tribe war | Off | Off = no tribe wars on PvE servers. |
| `bPvEAllowTribeWarCancel` | ✗ | bool = False | switch | On: PvE allow tribe war cancel | Off |  |
| `bPvEDisableFriendlyFire` | ✔ | bool = False | switch | On: PvE disable friendly fire | Off | When on, members of the same tribe deal no damage to each other (PvE). |
| `PvPZoneStructureDamageMultiplier` | ✔ | float = 6 | ↑ damage dealt | More damage | Less damage | If > 0, damage to structures in PvP zones x this. |

### Decay & auto-destroy

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `AutoDestroyDecayedDinos` | ✔ | bool = False |  | switch | On: auto destroy decayed dinos | Off | When on, creatures that have decayed (claimable) are destroyed automatically. |
| `AutoDestroyOldStructuresMultiplier` | ✔ | float = 0 |  | ↑ time before auto-destroy | Old structures are destroyed later | Destroyed sooner | Used only if > 0 (otherwise default); scales the auto-destroy period. |
| `DisableBurrowDecayTimers` | ✔ | bool = False |  | switch | Burrow decay timers off | Burrow decay timers on | Jackalope burrow structures: when on, their decay timers are skipped. |
| `FastDecayUnsnappedCoreStructures` | ✔ | bool = False |  | switch | On: fast decay unsnapped core structures | Off | When on, unsnapped core structures decay after FastDecayInterval. |
| `OnlyAutoDestroyCoreStructures` | ✔ | bool = False |  | switch | Only auto destroy core structures | Not restricted | When on, auto-destroy only targets core structures (foundations, pillars). |
| `OnlyDecayUnsnappedCoreStructures` | ✔ | bool = False |  | switch | Only decay unsnapped core structures | Not restricted | When on, only unsnapped core structures (foundations, pillars) auto-decay. |
| `PreventPlannedStructureDecayReset` | ✗ | bool = False |  | switch | Planned structure decay reset blocked | Planned structure decay reset allowed |  |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `CropDecaySpeedMultiplier` | ✔ × | float = 1 | ↑ crop decay speed | Crops rot faster | Crops rot slower |  |
| `bDisableBurrowDecayTimers` (also GUS) | ✔ | bool = False | switch | Burrow decay timers off | Burrow decay timers on | Jackalope burrow structures: when on, their decay timers are skipped. |
| `bDisableDinoDecayClaiming` | ✔ | bool = False | switch | Dino decay claiming off | Dino decay claiming on | When on, decayed creatures can't be claimed. |
| `FastDecayInterval` | ✔ | float = 43200 | ↑ fast decay interval | Longer | Shorter | Seconds; decay time becomes min(this, normal decay) for fast-decaying structures. |

### Structures & building

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `AllowBunkerModulesAboveGround` | ✔ | bool = False |  | switch | Bunker modules above ground allowed | Bunker modules above ground not allowed | When on, bunker modules may be placed above ground. |
| `AllowBunkerModulesInPreventionZones` | ✔ | bool = False |  | switch | Bunker modules in prevention zones allowed | Bunker modules in prevention zones not allowed | When off, bunker modules are refused inside prevention zones. |
| `AllowBunkersInPreventionZones` | ✔ | bool = False |  | switch | Bunkers in prevention zones allowed | Bunkers in prevention zones not allowed | When off, bunkers are refused inside prevention zones. |
| `AllowCrateSpawnsOnTopOfStructures` | ✔ | bool = False |  | switch | Crate spawns on top of structures allowed | Crate spawns on top of structures not allowed | When on, crates may land on structures. |
| `AllowDeprecatedStructures` | ✗ | bool = False |  | switch | Deprecated structures allowed | Deprecated structures not allowed |  |
| `AllowDinoAIInsideBunkers` | ✔ | bool = True | yes | switch | Dino AI inside bunkers allowed | Dino AI inside bunkers not allowed | When off, creature AI stops inside bunkers. |
| `AllowMultipleAttachedC4` | ✔ | bool = False |  | switch | Multiple attached c 4 allowed | Multiple attached c 4 not allowed | When off, C4 can't be stuck onto a creature/player that already has one. |
| `AllowRidingDinosInsideBunkers` | ✔ | bool = True | yes | switch | Riding dinos inside bunkers allowed | Riding dinos inside bunkers not allowed | Whether creatures can be ridden inside bunkers. |
| `AlwaysAllowStructurePickup` | ✔ | bool = True | yes | switch | On: always allow structure pickup | Off | When on, the pickup check skips the 'recently placed' window (StructurePickupTimeAfterPlacement): structures can always be picked up. |
| `AnchoredVesselCheckRadius` | ✗ | int = 0 |  | ↑ anchored vessel check radius | Larger area | Smaller area |  |
| `BunkerUnderHPThresholdDmgMultiplier` | ✔ | float = 0.05 | yes | ↑ damage to weakened bunkers | Bunkers under the threshold take more damage | Take less damage | Incoming damage × this once the bunker is under EnemyAccessBunkerHPThreshold. |
| `DestroyUnconnectedWaterPipes` | ✔ | bool = False |  | switch | On: destroy unconnected water pipes | Off | When on, unconnected water pipes are destroyed after their timer. |
| `EnableExtraStructurePreventionVolumes` | ✔ | bool = False |  | switch | Extra structure prevention volumes on | Extra structure prevention volumes off | When on, extra structure-prevention volumes are applied. |
| `EnableMeshBitingProtection` | ✔ | bool = True |  | switch | Mesh biting protection on | Mesh biting protection off | When on, creature bites through meshes are blocked. |
| `EnableStructureComponentMemOpt` | ✗ | bool = False |  | switch | Structure component mem opt on | Structure component mem opt off |  |
| `EnemyAccessBunkerHPThreshold` | ✔ | float = 0.25 | yes | ↑ HP fraction for enemy access | Enemies can enter bunkers at higher remaining HP | Bunker must be more damaged first | Enemies can access when health / max health ≤ this (fraction 0–1). |
| `ForceAllowCaveFlyers` | ✔ | bool = False |  | switch | Allow cave flyers forced | Not forced | When on, flyers are allowed in caves. |
| `ForceAllStructureLocking` | ✔ | bool = False |  | switch | All structure locking forced | Not forced | When on, the lock flags are set on all structures. |
| `ForceGachaUnhappyInCaves` | ✔ | bool = True | yes | switch | Gacha unhappy in caves forced | Not forced | When on, gachas are unhappy in caves. |
| `IgnoreLimitMaxStructuresInRangeTypeFlag` | ✔ | bool = False |  | switch | On: ignore limit max structures in range type flag | Off | When on, the build-limit count skips the per-structure-type exemption check. |
| `LimitBunkersPerTribe` | ✔ | bool = True | yes | switch | Bunkers per tribe limit on | No bunkers per tribe limit | When on, LimitBunkersPerTribeNum applies. |
| `LimitBunkersPerTribeNum` | ✔ | int = 3 | yes | ↑ limit bunkers per tribe num | Higher limit | Lower limit | Max bunkers per tribe (with LimitBunkersPerTribe). |
| `MaxAnchoredVesselsInRange` | ✗ | int = 0 |  | ↑ max anchored vessels in range | Larger area | Smaller area |  |
| `MaxGateFrameOnSaddles` | ✔ | int = -1 |  | ↑ max gate frame on saddles | Higher limit | Lower limit | Negative = no limit; 0 = none allowed; otherwise max gateways per platform saddle. |
| `MaxStructuresInSmallRadius` | ✔ | float = 0 |  | ↑ max structures in small radius | Larger area | Smaller area | If > 0, max structures within RadiusStructuresInSmallRadius. |
| `MinDistanceBetweenBunkers` | ✔ | float = 3000 | yes | ↑ min distance between bunkers | Larger area | Smaller area | If > 0, bunkers must be at least this far apart. |
| `NeedsPowerToActivateAquaticCompartments` | ✔ | bool = True | yes | switch | On: needs power to activate aquatic compartments | Off | When on, aquatic compartments need power to activate. |
| `OverrideStructurePlatformPrevention` | ✔ | bool = False |  | switch | On: override structure platform prevention | Off | When on, restricted structures can be built on platform saddles. |
| `PerPlatformMaxStructuresMultiplier` | ✔ | float = 1 | yes | ↑ platform saddle cap | More structures on platform saddles | Fewer | Platform saddle structure cap x this. |
| `PlatformSaddleBuildAreaBoundsMultiplier` | ◐ | float = 1 | yes | ↑ platform saddle build area bounds multiplier | Larger area | Smaller area | No map in the game sets this; its effect is not confirmed. |
| `PreventStructurePainting` | ✔ | bool = False |  | switch | Structure painting blocked | Structure painting allowed | When on, structures can't be painted. |
| `PreventTemplateOnSaddle` | ✔ | bool = False |  | switch | Template on saddle blocked | Template on saddle allowed | When on, structure templates can't be placed on saddles. |
| `RadiusStructuresInSmallRadius` | ✔ | float = 0 |  | ↑ radius structures in small radius | Larger area | Smaller area | Radius for MaxStructuresInSmallRadius. |
| `ServerEnableMeshChecking` | ✗ | bool = False |  | switch | On: server enable mesh checking | Off |  |
| `StructureDamageMultiplier` | ✔ | float = 1 |  | ↑ damage dealt | More damage | Less damage | Damage dealt by structures (turrets, traps) x this. |
| `StructurePickupHoldDuration` | ✗ | float = 0.5 | yes | ↑ hold time to pick up | Must hold the pickup key longer | Shorter hold |  |
| `StructurePickupTimeAfterPlacement` | ✗ | float = 30 | yes | ↑ pickup window | Structures can be picked up longer after placing | Shorter window |  |
| `StructureResistanceMultiplier` | ✔ × | float = 1 |  | ↑ damage structures take | Structures take more damage | Structures take less damage (tougher) |  |
| `TheMaxStructuresInRange` | ◐ | float = 10500 | yes | ↑ build limit | More structures allowed in range (×1.633 in effect) | Fewer structures allowed | The effective limit is this × 1.633 (default 10500 → 17146). Official servers force 10500. |
| `TribeLogDestroyedEnemyStructures` | ✔ | bool = False |  | switch | On: tribe log destroyed enemy structures | Off | When on, destroying an enemy structure is written to the tribe log. |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `bAllowBunkerModulesAboveGround` (also GUS) | ✔ | bool = False | switch | Bunker modules above ground allowed | Bunker modules above ground not allowed | When on, bunker modules may be placed above ground. |
| `bAllowBunkerModulesInPreventionZones` (also GUS) | ✔ | bool = False | switch | Bunker modules in prevention zones allowed | Bunker modules in prevention zones not allowed | When off, bunker modules are refused inside prevention zones. |
| `bAllowBunkersInPreventionZones` (also GUS) | ✔ | bool = False | switch | Bunkers in prevention zones allowed | Bunkers in prevention zones not allowed | When off, bunkers are refused inside prevention zones. |
| `bAllowDinoAIInsideBunkers` (also GUS) | ✔ | bool = True | switch | Dino AI inside bunkers allowed | Dino AI inside bunkers not allowed | When off, creature AI stops inside bunkers. |
| `bAllowPlatformSaddleMultiFloors` | ◐ | bool = False | switch | Platform saddle multi floors allowed | Platform saddle multi floors not allowed | No map in the game sets this; its effect is not confirmed. |
| `bAllowRidingDinosInsideBunkers` (also GUS) | ✔ | bool = True | switch | Riding dinos inside bunkers allowed | Riding dinos inside bunkers not allowed | Whether creatures can be ridden inside bunkers. |
| `BunkerUnderHPThresholdDmgMultiplier` (also GUS) | ✔ | float = 0.05 | ↑ damage to weakened bunkers | Bunkers under the threshold take more damage | Take less damage | Incoming damage × this once the bunker is under EnemyAccessBunkerHPThreshold. |
| `CityOutpostAllowMaxStructuresNearby` | ✔ | int = 5 | ↑ structures near city outposts | More structures allowed nearby | Fewer | Lost Colony city outposts: max player structures near a spawn point (above this, the outpost doesn't spawn there). |
| `CropPlotStackLimit` | ✔ | int = 100 | ↑ crop plot stack limit | Higher limit | Lower limit | Max crop plots stacked on each other (shown as "Can't stack more than N crop plots"). |
| `DinoTurretDamageMultiplier` | ✔ × | float = 1 | ↑ damage dealt | More damage | Less damage |  |
| `bDisableStructurePlacementCollision` | ✔ | bool = False | switch | Structure placement collision off | Structure placement collision on | When on, structure placement skips the world-geometry obstruction check. |
| `bDisableWirelessCraftingForStructures` | ✔ | bool = False | switch | Wireless crafting for structures off | Wireless crafting for structures on | When on, structures can't craft wirelessly. |
| `EnemyAccessBunkerHPThreshold` (also GUS) | ✔ | float = 0.25 | ↑ HP fraction for enemy access | Enemies can enter bunkers at higher remaining HP | Bunker must be more damaged first | Enemies can access when health / max health ≤ this (fraction 0–1). |
| `bFlyerPlatformAllowUnalignedDinoBasing` | ✔ | bool = True | switch | On: flyer platform allow unaligned dino basing | Off | When on, creatures may stand unaligned on flyer platform saddles. |
| `bForceAllowAllStructures` | ✗ | bool = False | switch | Allow all structures forced | Not forced |  |
| `bForceGachaUnhappyInCaves` (also GUS) | ✔ | bool = True | switch | Gacha unhappy in caves forced | Not forced | When on, gachas are unhappy in caves. |
| `bGenesisUseStructuresPreventionVolumes` | ✗ | bool = False | switch | On: genesis use structures prevention volumes | Off |  |
| `bHardLimitTurretsInRange` | ✔ | bool = False | switch | On: hard limit turrets in range | Off | Turret hard-limit checks (turrets, Spindles): when on, turrets over the limit stop working. |
| `bIgnoreStructuresPreventionVolumes` | ✔ | bool = False | switch | On: ignore structures prevention volumes | Off | When on, structure-prevention volumes are ignored. |
| `bLimitBunkersPerTribe` (also GUS) | ✔ | bool = True | switch | Bunkers per tribe limit on | No bunkers per tribe limit | When on, LimitBunkersPerTribeNum applies. |
| `LimitBunkersPerTribeNum` (also GUS) | ✔ | int = 3 | ↑ limit bunkers per tribe num | Higher limit | Lower limit | Max bunkers per tribe (with LimitBunkersPerTribe). |
| `LimitCloningChambersInRangeNum` | ✔ | int = 10 | ↑ limit cloning chambers in range num | Larger area | Smaller area | If ≥ 0, max chambers within LimitCloningChambersRange. |
| `LimitCloningChambersInTribeNum` | ✔ | int = 50 | ↑ limit cloning chambers in tribe num | Higher limit | Lower limit | If ≥ 0, max chambers per tribe. |
| `LimitCloningChambersRange` | ✔ | float = 10000 | ↑ limit cloning chambers range | Larger area | Smaller area | Radius for LimitCloningChambersInRangeNum. |
| `LimitGeneratorsNum` | ✔ | int = 3 | ↑ limit generators num | Higher limit | Lower limit | If > 0, max generators within LimitGeneratorsRange. |
| `LimitGeneratorsRange` | ✔ | float = 15000 | ↑ limit generators range | Larger area | Smaller area | If > 0, radius for LimitGeneratorsNum. |
| `LimitLinkedStoragesNum` | ✔ | int = 4 | ↑ limit linked storages num | Higher limit | Lower limit | Linked storage: if > 0, max linked storages. |
| `LimitReplicatorsPerSaddle` | ✔ | int = 1 | ↑ limit replicators per saddle | Higher limit | Lower limit | Tek Replicator: if ≥ 0, max replicators per saddle. |
| `bLimitTurretsInRange` | ✔ | bool = True | switch | Turrets in range limit on | No turrets in range limit | When on, LimitTurretsNum / LimitTurretsRange apply. |
| `LimitTurretsNum` | ✔ | int = 100 | ↑ limit turrets num | Higher limit | Lower limit | Max turrets within LimitTurretsRange (with bLimitTurretsInRange). |
| `LimitTurretsRange` | ✔ | float = 10000 | ↑ limit turrets range | Larger area | Smaller area | Radius for the turret limit (with bLimitTurretsInRange). |
| `MaxGateFrameOnSaddles` (also GUS) | ✔ | int = -1 | ↑ max gate frame on saddles | Higher limit | Lower limit | Negative = no limit; 0 = none allowed; otherwise max gateways per platform saddle. |
| `MaxPerTribePlatformSaddleStructureLimit` | ✗ | int = 0 | ↑ max per tribe platform saddle structure limit | Higher limit | Lower limit |  |
| `MaxPlatformSaddleStructureLimit` | ✔ | int = 75 | ↑ max platform saddle structure limit | Higher limit | Lower limit | If > 0, max structures on a platform saddle. |
| `MaxStructuresToAllowForPickupOverride` | ✗ | int = -1 | ↑ max structures to allow for pickup override | Higher limit | Lower limit |  |
| `MeshCheckingPercentageToFail` | ✔ | float = 0.2 | ↑ failure threshold | Anti-mesh check is more tolerant | Stricter | Parameter of the anti-meshing check. |
| `MeshCheckingRayDistance` | ✔ | float = 60000 | ↑ trace distance | Anti-mesh traces reach farther | Shorter traces | Parameter of the anti-meshing check (trace distance). |
| `MeshCheckingSubdivisons` | ✔ | float = 10 | ↑ trace count | More thorough anti-mesh check (more CPU) | Coarser check | Parameter of the anti-meshing check (trace count). |
| `MinDistanceBetweenBunkers` (also GUS) | ✔ | float = 3000 | ↑ min distance between bunkers | Larger area | Smaller area | If > 0, bunkers must be at least this far apart. |
| `bNeedsPowerToActivateAquaticCompartments` (also GUS) | ✔ | bool = True | switch | On: needs power to activate aquatic compartments | Off | When on, aquatic compartments need power to activate. |
| `OutpostAllowMaxStructuresNearby` | ✔ | int = 50 | ↑ structures near outposts | More structures allowed nearby | Fewer | Lost Colony outposts: max player structures near a spawn point (above this, the outpost doesn't spawn there). |
| `bPassiveDefensesDamageRiderlessDinos` | ✔ | bool = False | switch | On: passive defenses damage riderless dinos | Off | Spike walls: when on, they also damage riderless creatures. |
| `bPreventTemplateOnSaddle` (also GUS) | ✔ | bool = False | switch | Template on saddle blocked | Template on saddle allowed | When on, structure templates can't be placed on saddles. |
| `RTSProximityToEnemyStructureScale` | ✗ | float = 1 | ↑ RTS proximity to enemy structure scale | More / stronger | Less / weaker |  |
| `StructureDamageRepairCooldown` | ✔ lim | float = 180 | ↑ structure damage repair cooldown | Longer | Shorter |  |
| `UseStructurePreventionVolumeTagString` |  | string = — |  |  |  |  |

### Harvesting & resources

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `BloodforgeReinforceResourceCostMultiplier` | ✗ | float = 3 | yes | ↑ reinforcing cost | Reinforcing costs more | Costs less |  |
| `ClampResourceHarvestDamage` | ✔ | bool = False |  | switch | On: clamp resource harvest damage | Off | When on, damage dealt to a resource node is clamped to the node's remaining health. |
| `HarvestAmountMultiplier` | ✔ | float = 1 |  | ↑ resources per hit | More resources per hit | Fewer resources per hit | Resources per hit = count x this (a per-character modifier can override). |
| `HarvestHealthMultiplier` | ✔ ÷ | float = 1 |  | ↑ resource node HP | Nodes take more hits (more total yield per node) | Nodes break sooner |  |
| `ResourcesRespawnPeriodMultiplier` | ✔ | float = 1 |  | ↑ respawn time | Resources respawn slower | Resources respawn faster | Node min/max respawn time x this. |
| `StructurePreventResourceRadiusMultiplier` | ✔ × | float = 1 | yes | ↑ no-respawn radius | Structures block resource respawn over a wider area | Smaller blocked area |  |
| `UseOptimizedHarvestingHealth` | ✔ | bool = False |  | switch | Use optimized harvesting health | Don't use optimized harvesting health | When on, harvest yield comes from accumulated damage: every (per-item health ÷ HarvestHealthMultiplier) of damage gives one batch, remainder carried over. |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `BloodforgeReinforceResourceCostMultiplier` (also GUS) | ✗ | float = 3 | ↑ reinforcing cost | Reinforcing costs more | Costs less |  |
| `CropGrowthSpeedMultiplier` | ✗ | float = 1 | ↑ crop growth speed | Crops grow faster | Crops grow slower |  |
| `DinoHarvestingDamageMultiplier` | ✔ × | float = 3.2 | ↑ damage dealt | More damage | Less damage |  |
| `FishingLootQualityMultiplier` | ✔ | float = 1 | ↑ fishing loot quality | More / higher | Less / lower | Fishing rod / net: loot quality × this; 0 is special-cased. |
| `HarvestResourceItemAmountClassMultipliers` |  | list = (empty) |  |  |  |  |
| `MaxActiveResourceCaches` | ✔ | int = 8 | ↑ max active resource caches | Higher limit | Lower limit | Lost Colony resource caches: maximum active at once = this. |
| `MaxActiveResourceCachesSP` | ✔ | int = 8 | ↑ max active resource caches SP | Higher limit | Lower limit | Lost Colony resource caches (single player): maximum active at once = this. |
| `PlayerHarvestingDamageMultiplier` | ✔ × | float = 1 | ↑ damage dealt | More damage | Less damage |  |
| `ResourceNoReplenishRadiusPlayers` | ✗ | float = 1 | ↑ resource no replenish radius players | Larger area | Smaller area |  |
| `ResourceNoReplenishRadiusStructures` | ✔ × | float = 1 | ↑ resource no replenish radius structures | Larger area | Smaller area |  |

### Items, crafting & loot

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `BloodforgeReinforceExtraDurability` | ✔ | float = 0.3 | yes | ↑ durability from reinforcing | Reinforced items gain more durability | Less durability gain | Reinforced items get (1 + this) × durability. |
| `BloodforgeReinforceSpeedMultiplier` | ✔ | float = 0.1 | yes | ↑ reinforcing speed | Reinforcing is faster | Slower | Repair-speed multiplier while reinforcing at the Bloodforge. |
| `ClampItemSpoilingTimes` | ✔ | bool = False |  | switch | On: clamp item spoiling times | Off | Switches on clamping item spoil times. |
| `ClampItemStats` | ✔ | bool = False |  | switch | On: clamp item stats | Off | Switches on clamping item stats to ItemStatClamps. |
| `CosmoWeaponAmmoReloadAmount` | ✔ | float = 0 |  | ↑ ammo per reload | More ammo restored per reload | Less | Cosmo: if > 0, web ammo per reload = min(this, clip size). |
| `DinoStackingCheckRadius` | ✔ | float = 50 |  | ↑ dino stacking check radius | Larger area | Smaller area | Radius of the creature-stacking check. |
| `DinoStackingChecksPerFrame` | ✗ | int = 5 |  | ↑ stacking checks per frame | Stacking detection catches up faster (more CPU) | Fewer checks per frame |  |
| `DinoStackingMaxOverlap` | ✗ | int = 50 |  | ↑ allowed creature overlap | More stacked creatures tolerated | Fewer tolerated |  |
| `DisableDinoItemBlacklist` | ✗ | bool = False |  | switch | Dino item blacklist off | Dino item blacklist on |  |
| `EnableDinoStackingDetection` | ✗ | bool = False |  | switch | Dino stacking detection on | Dino stacking detection off |  |
| `ItemStackSizeMultiplier` | ✔ | float = 1 | yes | ↑ max stack size | Bigger stacks | Smaller stacks (never below 1) | Max stack = max(1, round(item max x this)) for items without a stack override and a max stack above 1. |
| `MaxBlueprintDinoLevel` | ✗ | int = 0 |  | ↑ max blueprint dino level | Higher limit | Lower limit |  |
| `MaxBlueprintDinoQuality` | ✗ | int = 0 |  | ↑ max blueprint dino quality | Higher limit | Lower limit |  |
| `MaxBlueprintItemQuality` | ✗ | int = 0 |  | ↑ max blueprint item quality | Higher limit | Lower limit |  |
| `MaxBlueprintScoutQuality` | ✗ | int = 0 |  | ↑ max blueprint scout quality | Higher limit | Lower limit |  |
| `MaxCosmoWeaponAmmo` | ✔ | float = 0 |  | ↑ max ammo | Higher ammo cap | Lower cap | Cosmo: if > 0, max web ammo = min(this, clip size). |
| `MaxTributeItems` | ✔ | int = 50 |  | ↑ max tribute items | Higher limit | Lower limit | Passed to the upload-limit check for items. |
| `OnLoadItemQualityRatingClamp` | ✔ | float = 0 |  | ↑ on load item quality rating clamp | Higher limit | Lower limit | If non-zero (with UseOnLoadClampItemQualityRating), items above this quality are clamped on load. |
| `PreventDownloadItems` | ✔ | bool = False |  | switch | Download items blocked | Download items allowed | When on, item downloads are refused. |
| `PreventDroppedItemPhysics` | ✔ | bool = True |  | switch | Dropped item physics blocked | Dropped item physics allowed | When on, dropped items don't simulate physics. |
| `PreventItemTraitEffects` | ✔ | bool = False |  | switch | Item trait effects blocked | Item trait effects allowed | When on, item traits have no effect. |
| `PreventNonClusterCharacterUploadsWithItems` | ✔ | bool = False |  | switch | Non cluster character uploads with items blocked | Non cluster character uploads with items allowed | When on, survivors carrying items can't be uploaded outside a cluster. |
| `PreventUploadItems` | ✔ | bool = False |  | switch | Upload items blocked | Upload items allowed | When on, item uploads are refused. |
| `RandomSupplyCratePoints` | ✔ | bool = False |  | switch | On: random supply crate points | Off | When on, supply crates use random spawn points; off = their fixed points. |
| `TributeItemExpirationSeconds` | ✔ | int = 86400 |  | ↑ tribute item expiration seconds | Longer | Shorter | If > 0, uploaded items expire after this many seconds. |
| `UseOnLoadClampItemQualityRating` | ✔ | bool = False |  | switch | Use on load clamp item quality rating | Don't use on load clamp item quality rating | When on, item quality is clamped on load (to OnLoadItemQualityRatingClamp). |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `bAllowCustomRecipes` | ✔ | bool = True | switch | Custom recipes allowed | Custom recipes not allowed | Custom-recipe items are only processed (recipe effects applied) when this is on. |
| `BloodforgeReinforceExtraDurability` (also GUS) | ✔ | float = 0.3 | ↑ durability from reinforcing | Reinforced items gain more durability | Less durability gain | Reinforced items get (1 + this) × durability. |
| `BloodforgeReinforceSpeedMultiplier` (also GUS) | ✔ | float = 0.1 | ↑ reinforcing speed | Reinforcing is faster | Slower | Repair-speed multiplier while reinforcing at the Bloodforge. |
| `ConfigOverrideItemCraftingCosts` |  | list = (empty) |  |  |  |  |
| `ConfigOverrideItemMaxQuantity` |  | list = (empty) |  |  |  |  |
| `ConfigOverrideSupplyCrateItems` |  | list = (empty) |  |  |  |  |
| `ConfigSettingsModifiedTraitEffectiveness` |  | map = (empty) |  |  |  |  |
| `ConfigSettingsModifiedTraitEffectiveness_Key` |  | name = — |  |  |  |  |
| `CosmoWeaponAmmoReloadAmount` (also GUS) | ✔ | int = 0 | ↑ ammo per reload | More ammo restored per reload | Less | Cosmo: if > 0, web ammo per reload = min(this, clip size). |
| `CraftingSkillBonusMultiplier` | ✔ | float = 1 | ↑ crafting skill bonus | More / higher | Less / lower | Multiplies the crafting-skill stat bonus. |
| `CustomRecipeEffectivenessMultiplier` | ✗ | float = 1 | ↑ custom recipe effectiveness | More / higher | Less / lower |  |
| `CustomRecipeSkillMultiplier` | ✗ | float = 1 | ↑ custom recipe skill | More / stronger | Less / weaker |  |
| `bDisableDefaultMapItemSets` | ✔ | bool = False | switch | Default map item sets off | Default map item sets on | When on, the map's default item sets are not used. |
| `bDisableDinoItemBlacklist` (also GUS) | ✗ | bool = False | switch | Dino item blacklist off | Dino item blacklist on |  |
| `bDisableLootCrates` | ✔ | bool = False | switch | Loot crates off | Loot crates on | When on, crates are not spawned. |
| `bDisableWirelessCrafting` | ✔ | bool = False | switch | Wireless crafting off | Wireless crafting on | When on, wireless crafting (pulling resources from nearby storage) is off. |
| `bDisableWirelessCraftingForDinos` | ✔ | bool = False | switch | Wireless crafting for dinos off | Wireless crafting for dinos on | When on, creatures can't craft wirelessly. |
| `bDisableWirelessCraftingForPlayers` | ✔ | bool = False | switch | Wireless crafting for players off | Wireless crafting for players on | When on, players can't craft wirelessly. |
| `ExcludeItemIndices` |  | list = (empty) |  |  |  |  |
| `bForceAllowAscensionItemDownloads` | ✔ | bool = True | switch | Allow ascension item downloads forced | Not forced | When on, ascension items are processed in downloads (allowed). |
| `bForceUseInventoryAppends` | ◐ | bool = False | switch | Use inventory appends forced | Not forced | When on, configured inventory additions are applied. |
| `FuelConsumptionIntervalMultiplier` | ✔ × | float = 1 | ↑ fuel duration | Fuel lasts longer | Fuel burns faster |  |
| `GlobalCorpseDecompositionTimeMultiplier` | ✔ | float = 0 | ↑ corpse lifetime | Corpses last longer | Corpses vanish sooner | Used only if > 0 (otherwise default); scales corpse lifetime. |
| `GlobalItemDecompositionTimeMultiplier` | ✔ | float = 0 | ↑ dropped-item lifetime | Dropped items last longer | Dropped items vanish sooner | Used only if > 0 (otherwise default); scales dropped-item lifetime. |
| `GlobalPoweredBatteryDurabilityDecreasePerSecond` | ✔ × | float = 3 | ↑ battery drain | Batteries drain faster | Drain slower |  |
| `GlobalSpoilingTimeMultiplier` | ✔ | float = 0 | ↑ spoil time | Items spoil slower | Items spoil faster | Spoil time = item spoil time x this; used only if > 0. |
| `ItemStackSizeMultiplier` (also GUS) | ✔ | float = 1 | ↑ max stack size | Bigger stacks | Smaller stacks (never below 1) | Max stack = max(1, round(item max x this)) for items without a stack override and a max stack above 1. |
| `ItemStatClamps` | ✔ | int[8] = all 0 | ↑ item stat clamps | Higher limit | Lower limit | Per-stat item stat cap; 0 = no clamp. |
| `LimitNonPlayerDroppedItemsCount` | ✔ | int = 0 | ↑ limit non player dropped items count | Higher limit | Lower limit | Max non-player dropped items within the range. |
| `LimitNonPlayerDroppedItemsRange` | ✔ | float = 0 | ↑ limit non player dropped items range | Larger area | Smaller area | Radius for counting non-player dropped items; 0 = off. |
| `MaxCosmoWeaponAmmo` (also GUS) | ✔ | int = 0 | ↑ max ammo | Higher ammo cap | Lower cap | Cosmo: if > 0, max web ammo = min(this, clip size). |
| `MinimumTimebetweeninventoryRetrieval` | ✗ | float = 3600 | ↑ minimum timebetweeninventory retrieval | Higher limit | Lower limit |  |
| `OnLoadItemQualityRatingClamp` (also GUS) | ✔ | float = 0 | ↑ on load item quality rating clamp | Higher limit | Lower limit | If non-zero (with UseOnLoadClampItemQualityRating), items above this quality are clamped on load. |
| `bPreventItemTraitEffects` (also GUS) | ✔ | bool = False | switch | Item trait effects blocked | Item trait effects allowed | When on, item traits have no effect. |
| `SingleplayerSettingsCorpseLifespanMultiplier` | ✗ | float = 6 | ↑ lifetime | Lasts longer | Disappears sooner |  |
| `SupplyCrateLootQualityMultiplier` | ✔ × | float = 1 | ↑ supply crate loot quality | More / higher | Less / lower |  |
| `UseCorpseLifeSpanMultiplier` | ✔ | float = 1 | ↑ corpse lifetime | Corpses last longer | Corpses vanish sooner | Corpse lifetime = species lifetime x this. |
| `bUseOnLoadClampItemQualityRating` (also GUS) | ✔ | bool = False | switch | Use on load clamp item quality rating | Don't use on load clamp item quality rating | When on, item quality is clamped on load (to OnLoadItemQualityRatingClamp). |
| `WirelessCraftingRangeOverride` | ✔ lim | float = 0 | ↑ wireless crafting range override | Larger area | Smaller area |  |

### Creatures, spawns & stats

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `AllowFlyingStaminaRecovery` | ✔ | bool = False |  | switch | Flying stamina recovery allowed | Flying stamina recovery not allowed | When on, flyers recover stamina while flying. |
| `AllowTekSuitPowersInGenesis` | ✔ | bool = False |  | switch | Tek suit powers in genesis allowed | Tek suit powers in genesis not allowed | When off, Tek suit powers are restricted in Genesis. |
| `AllowVariantDinoPacks` | ✔ | bool = False |  | switch | Variant dino packs allowed | Variant dino packs not allowed | When on, variant creature pack entries are used when spawning. |
| `DinoCharacterFoodDrainMultiplier` | ✔ | float = 1 |  | ↑ drain rate | Drains faster | Drains slower | Creature food drain x this. |
| `DinoCharacterHealthRecoveryMultiplier` | ✔ | float = 1 |  | ↑ recovery speed | Recovers faster | Recovers slower | Creature health recovery x this. |
| `DinoCharacterStaminaDrainMultiplier` | ✔ | float = 1 |  | ↑ drain rate | Drains faster | Drains slower | Creature stamina drain x this. |
| `DinoCountMultiplier` | ✔ | float = 1 |  | ↑ wild creature count | More wild creatures | Fewer wild creatures; ~0 disables some spawners | Clamped and used in the wild spawn-count formula; ~0 also switches a spawner flag off. |
| `DinoDamageMultiplier` | ✔ | float = 1 |  | ↑ damage dealt | More damage | Less damage | In this build it only affects the crab's attack; it has no effect on other creatures. |
| `DinoResistanceMultiplier` | ✔ × | float = 1 |  | ↑ damage creatures take | Creatures take more damage | Creatures take less damage (tougher) |  |
| `DisableTekLegsBoost` | ✔ | bool = False |  | switch | Tek legs boost off | Tek legs boost on | When on, the Tek run boost is blocked. |
| `EnablePlayerMoveThroughAlly` | ✗ | bool = False |  | switch | Player move through ally on | Player move through ally off |  |
| `EnablePlayerMoveThroughSleeping` | ✔ | bool = False |  | switch | Player move through sleeping on | Player move through sleeping off | When on, players can walk through sleeping players. |
| `bForceCanRideFliers` | ✗ | bool = False |  | switch | Can ride fliers forced | Not forced |  |
| `ForceResetWildDinos` | ✔ | bool = False |  | switch | Reset wild dinos forced | Not forced | When on, wild creatures are reset (respawned). |
| `NonPermanentDiseases` | ✔ | bool = False |  | switch | No n permanent diseases | N permanent diseases allowed | In the disease/buff processing loop: when on, buffs flagged as permanent are skipped, so diseases don't stay permanent. |
| `OxygenSwimSpeedStatMultiplier` | ✔ × | float = 1 | yes | ↑ swim speed from oxygen | Oxygen levels give more swim speed | Less swim speed |  |
| `PlayerCharacterFoodDrainMultiplier` | ✔ × | float = 1 |  | ↑ drain rate | Drains faster | Drains slower |  |
| `PlayerCharacterHealthRecoveryMultiplier` | ✔ × | float = 1 |  | ↑ recovery speed | Recovers faster | Recovers slower |  |
| `PlayerCharacterStaminaDrainMultiplier` | ✔ | float = 1 |  | ↑ drain rate | Drains faster | Drains slower | Player stamina drain x this. |
| `PlayerCharacterWaterDrainMultiplier` | ✔ × | float = 1 |  | ↑ drain rate | Drains faster | Drains slower |  |
| `PlayerDamageMultiplier` | ✔ | float = 1 |  | ↑ damage dealt | More damage | Less damage | Player-dealt damage x this. |
| `PlayerResistanceMultiplier` | ✔ × | float = 1 |  | ↑ damage players take | Players take more damage | Players take less damage (tougher) |  |
| `PreventDiseases` | ✔ | bool = False |  | switch | Diseases blocked | Diseases allowed | When on, there are no diseases. |
| `PreventOverworldBosses` | ✔ | bool = False |  | switch | Overworld bosses blocked | Overworld bosses allowed | World-boss spawner and King Kaiju: when on, overworld bosses are not spawned. |
| `PreventSpawnAnimations` | ✔ | bool = False |  | switch | Spawn animations blocked | Spawn animations allowed | When on, the spawn animation bits are cleared: players appear without the wake-up animation. |
| `RiderDinoCollision` | ✔ | bool = False |  | switch | On: rider dino collision | Off | Affects collision between riders and creatures. |
| `ServerAutoForceRespawnWildDinosInterval` | ✗ | float = 0 |  | ↑ server auto force respawn wild dinos interval | Longer | Shorter |  |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `bAllowVariantDinoPacks` (also GUS) | ✔ | bool = False | switch | Variant dino packs allowed | Variant dino packs not allowed | When on, variant creature pack entries are used when spawning. |
| `BaseTemperatureMultiplier` | ✔ × | float = 1 | ↑ base temperature | More / stronger | Less / weaker |  |
| `ConfigAddNPCSpawnEntriesContainer` |  | list = (empty) |  |  |  |  |
| `ConfigOverrideNPCSpawnEntriesContainer` |  | list = (empty) |  |  |  |  |
| `ConfigSubtractNPCSpawnEntriesContainer` |  | list = (empty) |  |  |  |  |
| `DinoClassDamageMultipliers` |  | list = (empty) |  |  |  |  |
| `DinoClassResistanceMultipliers` |  | list = (empty) |  |  |  |  |
| `DinoClassSpeedMultipliers` |  | list = (empty) |  |  |  |  |
| `DinoClassStaminaMultipliers` |  | list = (empty) |  |  |  |  |
| `DinoHairGrowthSpeedMultiplier` | ✔ | float = 1 | ↑ speed | Faster | Slower | Sheep wool / Archelon algae: time to full = base time / this; higher = faster regrowth. |
| `DinoSpawnWeightMultipliers` |  | list = (empty) |  |  |  |  |
| `bDisableDinoRiding` | ✔ | bool = False | switch | Dino riding off | Dino riding on | When on, creatures can't be ridden. |
| `bDisableTekLegsBoost` (also GUS) | ✔ | bool = False | switch | Tek legs boost off | Tek legs boost on | When on, the Tek run boost is blocked. |
| `bEnablePlayerMoveThroughAlly` (also GUS) | ✗ | bool = False | switch | Player move through ally on | Player move through ally off |  |
| `bEnablePlayerMoveThroughSleeping` (also GUS) | ✔ | bool = False | switch | Player move through sleeping on | Player move through sleeping off | When on, players can walk through sleeping players. |
| `GachasOverTribeLimitPenalty` | ✔ | float = 3 | ↑ penalty over gacha limit | Harsher penalty for gachas over the tribe limit | Milder | Gacha digestion time × this when the tribe has more gachas than NumGachasPerTribeForPenalty. |
| `HairGrowthSpeedMultiplier` | ✔ | float = 0 | ↑ hair growth speed | Hair grows faster | Hair grows slower; 0 or less stops it | Growth += dt x this / species time; 0 or less disables hair growth. |
| `MaxDinoBaseLevel` | ✔ | int = 0 | ↑ max dino base level | Higher limit | Lower limit | If > 0, wild creature base level is capped to this. |
| `MaxFallSpeedMultiplier` | ✔ × | float = 1 | ↑ speed | Faster | Slower |  |
| `NPCReplacements` |  | list = (empty) |  |  |  |  |
| `NumGachasPerTribeForPenalty` | ✔ | int = 100 | ↑ gachas before penalty | More gachas allowed before the penalty | Fewer | Gacha: if ≥ 0, the tribe's gacha count above this triggers GachasOverTribeLimitPenalty. |
| `PreventMimicDinoClassNames` |  | list = (empty) |  |  |  |  |
| `bPreventOverworldBosses` (also GUS) | ✔ | bool = False | switch | Overworld bosses blocked | Overworld bosses allowed | World-boss spawner and King Kaiju: when on, overworld bosses are not spawned. |
| `TamedDinoCharacterFoodDrainMultiplier` | ✗ | float = 1 | ↑ drain rate | Drains faster | Drains slower |  |
| `TamedDinoClassDamageMultipliers` |  | list = (empty) |  |  |  |  |
| `TamedDinoClassResistanceMultipliers` |  | list = (empty) |  |  |  |  |
| `TamedDinoClassSpeedMultipliers` |  | list = (empty) |  |  |  |  |
| `TamedDinoClassStaminaMultipliers` |  | list = (empty) |  |  |  |  |
| `TamedDinoTorporDrainMultiplier` | ✗ | float = 1 | ↑ drain rate | Drains faster | Drains slower |  |
| `WildDinoCharacterFoodDrainMultiplier` | ✗ | float = 1 | ↑ drain rate | Drains faster | Drains slower |  |
| `WildDinoTorporDrainMultiplier` | ✗ | float = 1 | ↑ drain rate | Drains faster | Drains slower |  |
| `WildFollowerSpawnChanceMultiplier` | ✔ | float = 1 | ↑ wild follower spawn chance | More / higher | Less / lower | Affects wild pack followers. |
| `WildFollowerSpawnCountMultiplier` | ✔ | float = 1 | ↑ wild follower spawn count | More / higher | Less / lower | Affects wild pack followers. |

### Transfers & cluster

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `CrossARKAllowForeignDinoDownloads` | ✔ | bool = False |  | switch | On: cross ark allow foreign dino downloads | Off | When on, creatures from other ARKs/maps may be downloaded. |
| `MaxTributeCharacters` | ✔ | int = 10 |  | ↑ max tribute characters | Higher limit | Lower limit | Passed to the upload-limit check for survivors. |
| `MaxTributeDinos` | ✔ | int = 20 |  | ↑ max tribute dinos | Higher limit | Lower limit | Passed to the upload-limit check for creatures. |
| `MinimumDinoReuploadInterval` | ✔ lim | float = 0 |  | ↑ minimum dino reupload interval | Higher limit | Lower limit |  |
| `NoTributeDownloads` | ✔ | bool = False |  | switch | No tribute downloads | Tribute downloads allowed | Stored inverted (downloads allowed); downloads proceed only when this is off. |
| `PreventDownloadDinos` | ✔ | bool = False |  | switch | Download dinos blocked | Download dinos allowed | When on, creature downloads are refused. |
| `PreventDownloadSurvivors` | ✔ | bool = False |  | switch | Download survivors blocked | Download survivors allowed | When on, survivor downloads are refused. |
| `PreventUploadDinos` | ✔ | bool = False |  | switch | Upload dinos blocked | Upload dinos allowed | When on, creature uploads are refused. |
| `PreventUploadSurvivors` | ✔ | bool = False |  | switch | Upload survivors blocked | Upload survivors allowed | When on, survivor uploads are refused. |
| `TributeCharacterExpirationSeconds` | ✔ | int = 0 |  | ↑ tribute character expiration seconds | Longer | Shorter | If > 0, uploaded survivors expire after this many seconds. |
| `TributeDinoExpirationSeconds` | ✔ | int = 86400 |  | ↑ tribute dino expiration seconds | Longer | Shorter | If > 0, uploaded creatures expire after this many seconds. |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `bDisableCustomFoldersInTributeInventories` | ✗ | bool = False | switch | Custom folders in tribute inventories off | Custom folders in tribute inventories on |  |
| `PreventTransferForClassNames` |  | list = (empty) |  |  |  |  |

### Tribes & players

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `AutoCreateTribes` | ✔ | bool = False |  | switch | On: auto create tribes | Off | When on, players are put into an automatically created tribe. |
| `EnableAFKKickPlayerCountPercent` | ✗ | float = 0 |  | ↑ player-count threshold for AFK kicks | AFK kicks start only when the server is fuller | AFK kicks start at lower population |  |
| `EnableDeathTeamSpectator` | ✔ | bool = False |  | switch | Death team spectator on | Death team spectator off | When on, dead players can spectate their team. |
| `bFilterCharacterNames` | ✔ | bool = False |  | switch | On: filter character names | Off | When on, survivor names are filtered for bad words. |
| `bFilterTribeNames` | ✔ | bool = False |  | switch | On: filter tribe names | Off | When on, tribe names are filtered for bad words. |
| `ImplantSuicideCD` | ✔ lim | float = 28800 | yes | ↑ implant suicide cooldown | Longer | Shorter |  |
| `KickIdlePlayersPeriod` | ✔ lim | float = 3600 | yes | ↑ idle time before kick | Idle players kicked later | Kicked sooner |  |
| `PreventTribeAlliances` | ✗ | bool = False |  | switch | Tribe alliances blocked | Tribe alliances allowed |  |
| `TribeNameChangeCooldown` | ✔ lim | float = 15 | yes | ↑ tribe name change cooldown | Longer | Shorter |  |
| `UseExclusiveList` | ✔ | bool = False |  | switch | Use exclusive list | Don't use exclusive list | When on, only players on the exclusive list may join. |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `bAllowDisablingSpectator` | ✔ | bool = True | switch | Disabling spectator allowed | Disabling spectator not allowed | When on, players may disable spectator mode. |
| `bDisableNonTribePinAccess` | ✔ | bool = False | switch | Non tribe pin access off | Non tribe pin access on | When on, PIN codes can't be used by players outside the tribe. |
| `bFreezeTribes` | ✗ | bool = False | switch | On: freeze tribes | Off |  |
| `MaxAlliancesPerTribe` | ✔ lim | int = 0 | ↑ max alliances per tribe | Higher limit | Lower limit |  |
| `MaxNumberOfPlayersInTribe` | ✔ | int = 0 | ↑ max number of players in tribe | Higher limit | Lower limit | Compared against tribe member count. |
| `MaxTribeLogs` | ✔ lim | int = 400 | ↑ max tribe logs | Higher limit | Lower limit |  |
| `MaxTribesPerAlliance` | ✔ lim | int = 0 | ↑ max tribes per alliance | Higher limit | Lower limit |  |
| `bSaveFormerTribeOwnerLog` | ✔ | bool = True | switch | On: save former tribe owner log | Off | When on, the former tribe's log is kept for the new owner. |
| `TimePeriodToHideDisconnectedPlayers` | ✔ | float = 0 | ↑ time before hiding | Logged-out bodies stay visible longer | Hidden sooner; 0 = never hidden | If > 0, a disconnected player's body is hidden after this many seconds. |
| `TribeSlotReuseCooldown` | ✔ lim | float = 0 | ↑ tribe slot reuse cooldown | Longer | Shorter |  |
| `TribeTowerBonusMultiplier` | ✔ | float = 1 | ↑ tribe tower bonus | More / higher | Less / lower | Tribe tower bonus × this. |

### Chat, UI & admin

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `AdminLogging` | ✔ | bool = False |  | switch | On: admin logging | Off | When on, admin commands are logged. |
| `AllowHideDamageSourceFromLogs` | ✔ | bool = True | yes | switch | Hide damage source from logs allowed | Hide damage source from logs not allowed | When on, the damage source can be hidden. |
| `AllowHitMarkers` | ✔ | bool = True | yes | switch | Hit markers allowed | Hit markers not allowed | When on, hit markers are sent to the attacker. |
| `AllowSharedConnections` | ✗ | bool = True |  | switch | Shared connections allowed | Shared connections not allowed |  |
| `AllowThirdPersonPlayer` | ✔ | bool = True | yes | switch | Third person player allowed | Third person player not allowed | Off restricts players to first-person view. |
| `AlwaysNotifyPlayerLeft` | ✗ | bool = False |  | switch | On: always notify player left | Off |  |
| `AutoSavePeriodMinutes` | ✔ | float = 15 | yes | ↑ time between saves | Saves less often (more progress lost on crash) | Saves more often | World save when elapsed minutes reach this. |
| `ChatLogFileSplitIntervalSeconds` | ✗ | int = 45 |  | ↑ chat log file split interval seconds | Longer | Shorter |  |
| `ChatLogFlushIntervalSeconds` | ✗ | int = 15 |  | ↑ chat log flush interval seconds | Longer | Shorter |  |
| `ChatLogMaxAgeInDays` | ✗ | int = 5 |  | ↑ chat log max age in days | Longer | Shorter |  |
| `DontAlwaysNotifyPlayerJoined` | ✗ | bool = False |  | switch | Don't always notify player joined | Always notify player joined |  |
| `DontRestoreBackup` | ✗ | bool = False |  | switch | Don't restore backup | Restore backup |  |
| `bFilterChat` | ✔ | bool = False |  | switch | On: filter chat | Off | When on, chat is filtered for bad words. |
| `GlobalVoiceChat` | ✔ | bool = False |  | switch | Voice heard server-wide | Proximity voice chat | True = voice is heard server-wide, False = proximity voice. |
| `LogChatMessages` | ✗ | bool = False |  | switch | On: log chat messages | Off |  |
| `ProximityChat` | ✔ | bool = False |  | switch | On: proximity chat | Off | When on, text chat is proximity-based. |
| `ProximityChatInnerRadius` | ✔ | float = 500 |  | ↑ proximity chat inner radius | Larger area | Smaller area | Voice range: full volume within this distance. |
| `ProximityChatOuterRadius` | ✔ | float = 5000 |  | ↑ proximity chat outer radius | Larger area | Smaller area | Voice range: inaudible beyond this distance. |
| `RCONEnabled` | ✗ | bool = False |  | switch | On: RCON enabled | Off |  |
| `RCONPort` | ✗ | int = 27020 | yes | port | RCON listens on this TCP port (set from RCON_PORT) | — |  |
| `RCONServerGameLogBuffer` | ✔ lim | float = 600 | yes | ↑ log lines kept for RCON | More game-log lines available to RCON | Fewer |  |
| `ServerCrosshair` | ✔ | bool = True | yes | switch | On: server crosshair | Off | When on, the crosshair is drawn. |
| `ServerForceNoHUD` | ✔ | bool = False |  | switch | On: server force no HUD | Off | When on, players get no HUD. |
| `ShowFloatingDamageText` | ✔ | bool = False |  | switch | Floating damage text shown | Floating damage text hidden | When on, damage numbers are sent to the attacker. |
| `ShowMapPlayerLocation` | ✔ | bool = True | yes | switch | Map player location shown | Map player location hidden | When on, players see their own position on the map. |
| `UpdateAllowedCheatersInterval` | ✗ | float = 600 |  | ↑ update allowed cheaters interval | Longer | Shorter |  |
| `UseCharacterTracker` | ✔ | bool = True |  | switch | Use character tracker | Don't use character tracker | Enables the character tracker (tracked creatures and waypoints). Can also be toggled at runtime by an admin command. |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `bAllowChatFromDeadNonAdmins` | ✔ | bool = True | switch | Chat from dead non admins allowed | Chat from dead non admins not allowed | When on, dead players (non-admins) may still chat. |
| `CheatTeleportLocations` |  | list = (TeleportName="testpoi1",TeleportLocation=(X=42069.0,Y=42069.0,Z=42069.0)) |  |  |  |  |
| `bDisableBuffSavePlayerDataOnSaveWorld` | ✔ | bool = False | switch | Buff save player data on save world off | Buff save player data on save world on | Companion buffs: when on, buff data isn't saved with the player on world save. |
| `bDisablePhotoMode` | ✔ | bool = False | switch | Photo mode off | Photo mode on | When on, photo mode is off. |
| `bDoAutomatedModValidationModeration` | ✗ | bool = False | switch | On: do automated mod validation moderation | Off |  |
| `bFailedWaterDinoSpawnLogEnabled` | ✔ | bool = False | switch | On: failed water dino spawn log enabled | Off | When on, failed water-creature spawns are logged. |
| `bGameplayLogEnabled` | ✗ | bool = False | switch | On: gameplay log enabled | Off |  |
| `ModValidationFailureWarnings` | ✗ | int = 1 | ↑ warnings before action | More warnings before mod-validation action | Fewer |  |
| `PhotoModeRangeLimit` | ✔ | float = 3000 | ↑ photo mode range limit | Larger area | Smaller area | Photo mode drone / targeting: max range; 0 = unlimited. |
| `RTSMaxRangeFromPlayerCharacterScale` | ✔ | float = 1 | ↑ RTS max range from player character scale | Larger area | Smaller area | Divides the RTS command range falloff; 0 or less disables the range check. |
| `RTSModeNumSelectableDinosScale` | ✗ | float = 1 | ↑ RTS mode num selectable dinos scale | More / stronger | Less / weaker |  |
| `bServerGameLogEnabled` | ✔ | bool = False | switch | On: server game log enabled | Off | When on, events are written to the server game log (also on with -servergamelog). |
| `bShowCreativeMode` | ✗ | bool = False | switch | Creative mode shown | Creative mode hidden |  |

### World, time & events

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `DayCycleSpeedScale` | ✔ | float = 1 |  | ↑ day/night cycle speed | Days pass faster | Days pass slower | Time speed = the map's own day-cycle factor × this × the day or night factor. |
| `DayTimeSpeedScale` | ✔ | float = 1 |  | ↑ daytime speed | Daytime passes faster (shorter days) | Longer days | Daytime speed = map's DayTimeSpeedScale × this (× DayCycleSpeedScale). Maps such as The Center set their own factor. |
| `DisableWeatherFog` | ✗ | bool = False |  | switch | Weather fog off | Weather fog on |  |
| `ElementalIslandShutdownFrequencyMultiplier` | ✗ | float = 1 | yes | ↑ elemental island shutdown frequency | More / stronger | Less / weaker |  |
| `ExtinctionEventTimeInterval` | ✔ ÷ | float = 0 |  | ↑ extinction event time interval | Longer | Shorter |  |
| `NightTimeSpeedScale` | ✔ | float = 1 |  | ↑ night speed | Night passes faster (shorter nights) | Longer nights | Night speed = map's NightTimeSpeedScale × this (× DayCycleSpeedScale). Maps such as Scorched Earth set their own factor. |
| `OverrideStartTime` | ✔ | bool = False |  | switch | Server starts at StartTimeHour | Map default start time | When on, StartTimeHour is used as the starting hour; when off it is ignored. |
| `StartTimeHour` | ✔ | int = -1 | yes | ↑ starting hour | Server starts later in the day (with OverrideStartTime) | Earlier | Only applies when OverrideStartTime is on; otherwise the map's start time is used. |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `bDisableWorldBuffs` | ✗ | bool = False | switch | World buffs off | World buffs on |  |
| `bEnableWorldBuffScaling` | ✔ | bool = False | switch | World buff scaling on | World buff scaling off | When on, world buff scaling (WorldBuffScalingEfficacy) is applied; off = 0. |
| `ExtinctionEventTimeInterval` (also GUS) | ✔ ÷ | float = 0 | ↑ extinction event time interval | Longer | Shorter |  |
| `NextExtinctionEventUTC` | ✗ | int = 0 | timestamp | Next Extinction event time (UTC, set by the game) | — |  |
| `ValgueroMemorialEntries` |  | string = — |  |  |  |  |
| `WorldBuffScalingEfficacy` | ✔ × | float = 0 | ↑ world buff strength | World buffs scale more strongly | Weaker |  |

### Missions, outposts & hexagons

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|---|
| `EnforceMilestoneTaskOrder` | ✗ | bool = False |  | switch | On: enforce milestone task order | Off |  |
| `MaxHexagonsPerCharacter` | ✔ | int = 2000000000 |  | ↑ max hexagons per character | Higher limit | Lower limit | Negative = no cap; otherwise hexagon balance is clamped to this. |
| `MaxHexagonsToAward` | ✔ | int = -1 |  | ↑ max hexagons to award | Higher limit | Lower limit | Negative = no cap; otherwise caps hexagons awarded. |
| `OfficialDisableGenesisMissions` | ✗ | bool = False |  | switch | On: official disable genesis missions | Off |  |
| `OutpostSigilRewardMultiplier` | ✔ | float = 1 | yes | ↑ outpost sigil reward | More / higher | Less / lower | Lost Colony outposts: sigils distributed × this. |
| `OverrideSecondsUntilBuriedTreasureAutoReveals` | ✔ | int = 1209600 | yes | ↑ override seconds until buried treasure auto reveals | Longer | Shorter | If > 0, seconds until a buried treasure reveals itself. |

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False | Notes |
|---|---|---|---|---|---|---|
| `BaseHexagonRewardMultiplier` | ✔ × | float = 1 | ↑ base hexagon reward | More / higher | Less / lower |  |
| `bDisableGenesisMissions` | ✔ | bool = False | switch | Genesis missions off | Genesis missions on | When on, Genesis missions are skipped. |
| `bDisableHexagonStore` | ✔ | bool = False | switch | Hexagon store off | Hexagon store on | HLNA companion: when on, the Hexagon store entries are disabled. |
| `bDontEnforceMilestoneTaskOrder` | ✗ | bool = False | switch | Don't enforce milestone task order | Enforce milestone task order |  |
| `HexagonCostMultiplier` | ✔ | float = 1 | ↑ cost | More expensive | Cheaper | Hexagon store cost x this. |
| `HexagonRewardMultiplier` | ✔ | float = 1 | ↑ hexagon reward | More / higher | Less / lower | Hexagon reward x this. |
| `bHexStoreAllowOnlyEngramTradeOption` | ✔ | bool = False | switch | On: hex store allow only engram trade option | Off | When on, the Hexagon store only offers items flagged as engram trades. |
| `MaxActiveCityOutposts` | ✔ | int = 8 | ↑ max active city outposts | Higher limit | Lower limit | Lost Colony city outposts: maximum active at once = this. |
| `MaxActiveCityOutpostsSP` | ✔ | int = 8 | ↑ max active city outposts SP | Higher limit | Lower limit | Lost Colony city outposts (single player): maximum active at once = this. |
| `MaxActiveGenesisLandOutposts` | ✔ | int = 25 | ↑ max active genesis land outposts | Higher limit | Lower limit | Genesis land outposts: maximum active at once = this. |
| `MaxActiveOutposts` | ✔ | int = 14 | ↑ max active outposts | Higher limit | Lower limit | Genesis water and Lost Colony outposts: maximum active at once = this. |
| `MaxActiveOutpostsSP` | ✔ | int = 14 | ↑ max active outposts SP | Higher limit | Lower limit | Outposts (single player): maximum active at once = this. |
| `MaxHexagonsPerCharacter` (also GUS) | ✔ | int = 2000000000 | ↑ max hexagons per character | Higher limit | Lower limit | Negative = no cap; otherwise hexagon balance is clamped to this. |
| `MaxHexagonsToAward` (also GUS) | ✔ | int = -1 | ↑ max hexagons to award | Higher limit | Lower limit | Negative = no cap; otherwise caps hexagons awarded. |
| `bOutpostsNoInitialDinosSP` | ✔ | bool = False | switch | On: outposts no initial dinos SP | Off | Lost Colony outposts (single player): when on, no initial creatures spawn. |


## Other sections

| File | Section | Keys |
|---|---|---|
| `GameUserSettings.ini` | `[SessionSettings]` | `SessionName` (set from `SESSION_NAME`) |
| `GameUserSettings.ini` | `[/Script/Engine.GameSession]` | `MaxPlayers` (default 70) |
| `GameUserSettings.ini` | `[MessageOfTheDay]` | `Message`, `Duration` |
