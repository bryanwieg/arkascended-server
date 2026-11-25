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
- **†**: the key has a note in the **Notes** list under its table: special
  values (for example "0 = no limit"), inverted switches and other details.
- **Load test**: every number and on/off key was set to a non-default value
  on a test server and confirmed to load from the file and section shown,
  unless its Notes say otherwise. Strings and lists were not tested.
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

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `ActiveEvent` |  | string = (empty) |  |  |  |  |
| `ActiveMapMod` |  | string = (empty) |  |  |  |  |
| `ActiveMods` |  | string = (empty) |  |  |  |  |
| `ActiveTotalConversion` |  | string = (empty) |  |  |  |  |
| `AdminListURL` |  | string = (empty) |  |  |  |  |
| `AutoRestartIntervalSeconds` † | ✗ | float = 0 |  | ↑ auto restart interval seconds | Longer | Shorter |
| `BadWordListURL` |  | string = (empty) |  |  |  |  |
| `BadWordWhiteListURL` |  | string = (empty) |  |  |  |  |
| `BanListURL` |  | string = (empty) |  |  |  |  |
| `BGOverridePath` |  | string = (empty) |  |  |  |  |
| `ClassLiveTunableConfigUrl` |  | string = (empty) |  |  |  |  |
| `CosmeticWhitelistOverride` |  | string = (empty) |  |  |  |  |
| `CustomDynamicConfigUrl` |  | string = (empty) |  |  |  |  |
| `CustomDynamicUndermeshConfigUrl` |  | string = (empty) |  |  |  |  |
| `CustomLiveTuningUrl` |  | string = (empty) |  |  |  |  |
| `NetModBanlistOverride` |  | string = (empty) |  |  |  |  |
| `ServerAdminPassword` |  | string = (empty) |  |  |  |  |
| `ServerPassword` |  | string = (empty) |  |  |  |  |
| `SpectatorPassword` |  | string = (empty) |  |  |  |  |
| `WorldModifier` |  | string = (empty) |  |  |  |  |

Notes:

- `AutoRestartIntervalSeconds`: Counts down from server start (seconds).

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `LiveTuningFileName` |  | string = — |  |  |  |

### Cryopods & companions

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `AllowCryoFridgeOnSaddle` † | ✔ | bool = False |  | switch | Cryo fridge on saddle allowed | Cryo fridge on saddle not allowed |
| `ArmadoggoDeathCooldown` † | ✔ | float = 0 |  | ↑ armadoggo death cooldown | Longer | Shorter |
| `CompanionsDeathCooldown` † | ✔ | float = 0 |  | ↑ companions death cooldown | Longer | Shorter |
| `CryoHospitalHoursToDrainTorpor` † | ✔ | float = 1 | yes | ↑ drain rate | Drains faster | Drains slower |
| `CryoHospitalHoursToRegenFood` † | ✔ | float = 24 | yes | ↑ cryo hospital hours to regen food | Longer | Shorter |
| `CryoHospitalHoursToRegenHP` † | ✔ | float = 1 | yes | ↑ cryo hospital hours to regen HP | Longer | Shorter |
| `CryoHospitalMatingCooldownReduction` † | ✔ | float = 2 | yes | ↑ mating cooldown reduction | Mating cooldown shrinks faster in the Cryo Hospital | Slower |
| `CryopodFridgeCooldownTime` † | ✔ | float = 90 |  | ↑ cryopod fridge cooldown time | Longer | Shorter |
| `CryopodNerfDamageMult` † | ✔ | float = 0 |  | ↑ damage dealt after uncryo | Freshly released creatures deal more damage (milder nerf) | Deal less damage (harsher nerf) |
| `CryopodNerfDuration` † | ✔ | float = 0 |  | ↑ nerf duration | The post-cryo nerf lasts longer | Shorter |
| `CryopodNerfIncomingDamageMult` † | ✗ | float = 0 |  | ↑ damage taken after uncryo | Freshly released creatures take more damage | Take less damage |
| `CryopodNerfIncomingDamageMultPercent` † | ✔ | float = 0 |  | ↑ damage taken after uncryo | Freshly released creatures take more damage | Take less damage |
| `DisableCryopodEnemyCheck` † | ✔ | bool = False |  | switch | Cryopod enemy check off | Cryopod enemy check on |
| `DisableCryopodFridgeRequirement` † | ✔ | bool = False |  | switch | Cryopod fridge requirement off | Cryopod fridge requirement on |
| `EnableCryopodNerf` † | ✔ | bool = False |  | switch | Cryopod nerf on | Cryopod nerf off |
| `EnableCryoSicknessPVE` † | ✔ | bool = False |  | switch | Cryo sickness PvE on | Cryo sickness PvE off |
| `YoungIceFoxDeathCooldown` † | ✔ | float = 0 |  | ↑ young ice fox death cooldown | Longer | Shorter |

Notes:

- `AllowCryoFridgeOnSaddle`: When off, Cryofridges can't be placed on saddles.
- `ArmadoggoDeathCooldown`: If > 0, the companion's post-death cryo duration (seconds); 0 = creature default.
- `CompanionsDeathCooldown`: If > 0, the companion's post-death cryo duration (seconds); 0 = creature default.
- `CryoHospitalHoursToDrainTorpor`: In the Cryo Hospital, torpor drain rate = full amount / this hours; higher = slower.
- `CryoHospitalHoursToRegenFood`: In the Cryo Hospital, food regen rate = full amount / this hours; higher = slower.
- `CryoHospitalHoursToRegenHP`: In the Cryo Hospital, health regen rate = full amount / this hours; higher = slower.
- `CryoHospitalMatingCooldownReduction`: On release, the mating cooldown is reduced by time spent in the Cryo Hospital × this.
- `CryopodFridgeCooldownTime`: Seconds before a cryopod can be taken out of a Cryofridge again.
- `CryopodNerfDamageMult`: Damage multiplier during the cryopod nerf.
- `CryopodNerfDuration`: Duration of the cryopod nerf (seconds).
- `CryopodNerfIncomingDamageMult`: Load not confirmed.
- `CryopodNerfIncomingDamageMultPercent`: Incoming-damage multiplier during the cryopod nerf.
- `DisableCryopodEnemyCheck`: When on, the 'enemies nearby' check is skipped.
- `DisableCryopodFridgeRequirement`: When on, cryopods don't need a Cryofridge/powered storage.
- `EnableCryopodNerf`: When on, released creatures get the cryopod nerf (CryopodNerfDuration, CryopodNerfDamageMult, CryopodNerfIncomingDamageMultPercent).
- `EnableCryoSicknessPVE`: When on, cryo sickness applies on PvE too.
- `YoungIceFoxDeathCooldown`: If > 0, post-death cryo duration (seconds); 0 = creature default.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `bAllowCryoFridgeOnSaddle` (also GUS) † | ✔ | bool = False | switch | Cryo fridge on saddle allowed | Cryo fridge on saddle not allowed |
| `ArmadoggoDeathCooldown` (also GUS) † | ✔ | float = 0 | ↑ armadoggo death cooldown | Longer | Shorter |
| `CompanionsDeathCooldown` (also GUS) † | ✔ | float = 0 | ↑ companions death cooldown | Longer | Shorter |
| `CryoHospitalHoursToDrainTorpor` (also GUS) † | ✔ | float = 1 | ↑ drain rate | Drains faster | Drains slower |
| `CryoHospitalHoursToRegenFood` (also GUS) † | ✔ | float = 24 | ↑ cryo hospital hours to regen food | Longer | Shorter |
| `CryoHospitalHoursToRegenHP` (also GUS) † | ✔ | float = 1 | ↑ cryo hospital hours to regen HP | Longer | Shorter |
| `CryoHospitalMatingCooldownReduction` (also GUS) † | ✔ | float = 2 | ↑ mating cooldown reduction | Mating cooldown shrinks faster in the Cryo Hospital | Slower |
| `CryopodFridgeCooldownTime` (also GUS) † | ✔ | float = 90 | ↑ cryopod fridge cooldown time | Longer | Shorter |
| `bDisableCryopodEnemyCheck` (also GUS) † | ✔ | bool = False | switch | Cryopod enemy check off | Cryopod enemy check on |
| `bDisableCryopodFridgeRequirement` (also GUS) † | ✔ | bool = False | switch | Cryopod fridge requirement off | Cryopod fridge requirement on |
| `YoungIceFoxDeathCooldown` (also GUS) † | ✔ | float = 0 | ↑ young ice fox death cooldown | Longer | Shorter |

Notes:

- `bAllowCryoFridgeOnSaddle`: When off, Cryofridges can't be placed on saddles.
- `ArmadoggoDeathCooldown`: If > 0, the companion's post-death cryo duration (seconds); 0 = creature default.
- `CompanionsDeathCooldown`: If > 0, the companion's post-death cryo duration (seconds); 0 = creature default.
- `CryoHospitalHoursToDrainTorpor`: In the Cryo Hospital, torpor drain rate = full amount / this hours; higher = slower.
- `CryoHospitalHoursToRegenFood`: In the Cryo Hospital, food regen rate = full amount / this hours; higher = slower.
- `CryoHospitalHoursToRegenHP`: In the Cryo Hospital, health regen rate = full amount / this hours; higher = slower.
- `CryoHospitalMatingCooldownReduction`: On release, the mating cooldown is reduced by time spent in the Cryo Hospital × this.
- `CryopodFridgeCooldownTime`: Seconds before a cryopod can be taken out of a Cryofridge again.
- `bDisableCryopodEnemyCheck`: When on, the 'enemies nearby' check is skipped.
- `bDisableCryopodFridgeRequirement`: When on, cryopods don't need a Cryofridge/powered storage.
- `YoungIceFoxDeathCooldown`: If > 0, post-death cryo duration (seconds); 0 = creature default.

### Breeding & imprinting

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `AllowAnyoneBabyImprintCuddle` † | ✔ | bool = False |  | switch | Anyone baby imprint cuddle allowed | Anyone baby imprint cuddle not allowed |
| `DisableGeneTraits` † | ✔ | bool = False |  | switch | Gene traits off | Gene traits on |
| `DisableImprintDinoBuff` † | ✔ | bool = False |  | switch | Imprint dino buff off | Imprint dino buff on |
| `FreezeReaperPregnancy` † | ✗ | bool = False |  | switch | On: freeze reaper pregnancy | Off |
| `PreventMateBoost` † | ✔ | bool = False |  | switch | Mate boost blocked | Mate boost allowed |
| `PreventTekChemBenchBabies` † | ✔ | bool = False |  | switch | Tek chem bench babies blocked | Tek chem bench babies allowed |

Notes:

- `AllowAnyoneBabyImprintCuddle`: When on, anyone may do imprint cuddles, not just the imprinter.
- `DisableGeneTraits`: When on, wild creatures spawn without gene traits. Can also be toggled at runtime by an admin command.
- `DisableImprintDinoBuff`: When on, the imprint damage bonus is not applied.
- `FreezeReaperPregnancy`: Load not confirmed.
- `PreventMateBoost`: When on, the mate-boost stack timer is skipped: no mate boost.
- `PreventTekChemBenchBabies`: Tek chemistry bench: when on, its 'babyfy creature' option is refused.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `BabyCuddleGracePeriodMultiplier` | ✔ × | float = 1 | ↑ grace period | More time to cuddle before imprint is lost | Less time to cuddle |
| `BabyCuddleIntervalMultiplier` † | ✔ | float = 1 | ↑ time between cuddles | Longer gap between imprint cuddles | Cuddles come more often (lower with BabyMatureSpeedMultiplier) |
| `BabyCuddleLoseImprintQualitySpeedMultiplier` | ✔ × | float = 1 | ↑ imprint loss speed | Imprint is lost faster after a missed cuddle | Imprint is lost slower |
| `BabyFoodConsumptionSpeedMultiplier` † | ✔ | float = 1 | ↑ baby food use | Babies eat faster | Babies eat slower |
| `BabyImprintAmountMultiplier` | ✗ | float = 1 | ↑ imprint per cuddle | More imprint % per cuddle | Less imprint % per cuddle |
| `BabyImprintingStatScaleMultiplier` | ✔ × | float = 1 | ↑ imprint stat bonus | Bigger stat bonus from imprinting | Smaller imprint bonus |
| `BabyMatureSpeedMultiplier` † | ✔ | float = 1 | ↑ maturation speed | Babies grow up faster | Babies grow up slower |
| `EggHatchSpeedMultiplier` † | ✔ | float = 1 | ↑ hatch speed | Eggs hatch faster | Eggs hatch slower |
| `ImprintLimit` | ✔ lim | float = -1 | ↑ imprint limit | Higher limit | Lower limit |
| `LayEggIntervalMultiplier` | ✔ × | float = 1 | ↑ time between eggs | Eggs laid less often | Eggs laid more often |
| `MatingIntervalMultiplier` † | ✔ | float = 1 | ↑ wait between matings | Longer wait between matings | Mating available sooner |
| `MatingSpeedMultiplier` | ✔ × | float = 1 | ↑ speed | Faster | Slower |
| `MutagenLevelBoost` † | ✔ | int[12] = [0]=5, [1]=5, [7]=5, [8]=5; others 0 | ↑ levels per stat from Mutagen | More levels added per stat | Fewer |
| `MutagenLevelBoost_Bred` † | ✔ | int[12] = [0]=1, [1]=1, [7]=1, [8]=1; others 0 | ↑ levels per stat from Mutagen (bred) | More levels added per stat | Fewer |
| `PoopIntervalMultiplier` † | ✔ | float = 1 | ↑ time between poops | Poops less often | Poops more often |
| `bPreventTekChemBenchBabies` (also GUS) † | ✔ | bool = False | switch | Tek chem bench babies blocked | Tek chem bench babies allowed |
| `ServerGeneTraitSpawnRateMultiplier` † | ✔ | float = 1 | ↑ server gene trait spawn rate | More / higher | Less / lower |

Notes:

- `BabyCuddleIntervalMultiplier`: Cuddle interval = random(min, max) of the species interval, each multiplied by this.
- `BabyFoodConsumptionSpeedMultiplier`: Baby food drain multiplied by this.
- `BabyMatureSpeedMultiplier`: Maturation += dt x rate x this (capped at 1.0).
- `EggHatchSpeedMultiplier`: Incubation += dt x rate x this.
- `MatingIntervalMultiplier`: Next mating time = now + interval x this.
- `MutagenLevelBoost`: Levels added per stat (index) by Mutagen on non-bred creatures.
- `MutagenLevelBoost_Bred`: Levels added per stat (index) by Mutagen on bred creatures.
- `PoopIntervalMultiplier`: Timer += dt / this; the next interval is also multiplied by this.
- `bPreventTekChemBenchBabies`: Tek chemistry bench: when on, its 'babyfy creature' option is refused.
- `ServerGeneTraitSpawnRateMultiplier`: Number of gene traits rolled x this.

### Taming & tame limits

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `bCheckSoftTameLimitOnTick` † | ✔ | bool = False |  | switch | On: check soft tame limit on tick | Off |
| `CheckSoftTameLimitOnTick` † | ✔ | bool = False |  | switch | On: check soft tame limit on tick | Off |
| `DestroyTamesOverTheSoftTameLimit` † | ✔ | bool = False |  | switch | On: destroy tames over the soft tame limit | Off |
| `bEnableServerDestroyTamesAboveSoftTameLimit` | ○ | bool = False |  | switch | Server destroy tames above soft tame limit on | Server destroy tames above soft tame limit off |
| `ForceExploitedTameDeletion` † | ✔ | bool = False |  | switch | Exploited tame deletion forced | Not forced |
| `MaxPersonalTamedDinos` † | ✔ | int = 250 |  | ↑ personal tame cap | More tames per tribe | Fewer tames per tribe; 0 = no limit |
| `MaxTamedDinos` † | ✔ | float = 5000 | yes | ↑ server tame cap | More tames allowed server-wide | Fewer tames allowed |
| `MaxTamedDinos_SoftTameLimit` | ✔ lim | int = 5000 | yes | ↑ max tamed dinos soft tame limit | Higher limit | Lower limit |
| `MaxTamedDinos_SoftTameLimit_CountdownForDeletionDuration` † | ✔ | int = 604800 | yes | ↑ time before over-limit tames are deleted | Over-limit tames are deleted later | Deleted sooner |
| `PersonalTamedDinosSaddleStructureCost` † | ✔ | int = 0 |  | ↑ tame-limit cost of saddle structures | Saddle structures use more of the personal tame limit | Use less; 0 = free |
| `TamingSpeedMultiplier` | ✔ × | float = 1 |  | ↑ taming speed | Faster taming | Slower taming |

Notes:

- `bCheckSoftTameLimitOnTick`: When on, the soft tame limit is checked continuously instead of only through the deletion countdown.
- `CheckSoftTameLimitOnTick`: When on, the soft tame limit is checked continuously instead of only through the deletion countdown.
- `DestroyTamesOverTheSoftTameLimit`: When on, the soft tame-limit deletion countdown runs (tames over MaxTamedDinos_SoftTameLimit get destroyed).
- `ForceExploitedTameDeletion`: When on, tames flagged as exploited are deleted.
- `MaxPersonalTamedDinos`: 0 = no limit; otherwise per-tribe personal tame cap.
- `MaxTamedDinos`: Server-wide tame cap.
- `MaxTamedDinos_SoftTameLimit_CountdownForDeletionDuration`: Seconds before tames over the soft limit are deleted.
- `PersonalTamedDinosSaddleStructureCost`: Structures on a saddle count toward the personal tame limit at this cost.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `bAllowUnclaimDinos` † | ✔ | bool = True | switch | Unclaim dinos allowed | Unclaim dinos not allowed |
| `DestroyTamesOverLevelClamp` | ✔ lim | int = 0 | ↑ destroy tames over level clamp | Higher limit | Lower limit |
| `bDisableDefaultDinoTaming` † | ✔ | bool = False | switch | Default dino taming off | Default dino taming on |
| `bDisableDinoTaming` † | ✔ | bool = False | switch | Dino taming off | Dino taming on |
| `bForceExploitedTameDeletion` (also GUS) † | ✔ | bool = False | switch | Exploited tame deletion forced | Not forced |
| `PassiveTameIntervalMultiplier` | ✔ × | float = 1 | ↑ time between feedings | Passive tames need feeding less often (slower tame) | Feedings come sooner (faster tame) |
| `PreventDinoTameClassNames` |  | list = (empty) |  |  |  |
| `PreventDisableDefaultDinoTameClassNames` |  | list = (empty) |  |  |  |
| `bUseTameLimitForStructuresOnly` † | ✔ | bool = False | switch | Use tame limit for structures only | Don't use tame limit for structures only |

Notes:

- `bAllowUnclaimDinos`: Off = creatures cannot be unclaimed.
- `bDisableDefaultDinoTaming`: When on, normal taming is disabled for every creature class except those in PreventDisableDefaultDinoTameClassNames.
- `bDisableDinoTaming`: When on, creatures gain no taming progress: no taming.
- `bForceExploitedTameDeletion`: When on, tames flagged as exploited are deleted.
- `bUseTameLimitForStructuresOnly`: When on, the personal tame limit (MaxPersonalTamedDinos) no longer counts tames; only saddle structures count, at PersonalTamedDinosSaddleStructureCost each.

### XP & levels

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `DifficultyOffset` † | ✔ | float = 0 |  | ↑ wild levels | Higher wild levels (toward the map maximum) | Lower wild levels; 0 = difficulty 1.0 |
| `OverrideOfficialDifficulty` † | ✔ | float = 0 |  | ↑ wild levels | Higher max wild level (30 × value) | Lower; 0 = use DifficultyOffset |
| `XPMultiplier` | ✔ × | float = 1 |  | ↑ XP gained | More XP from everything | Less XP |

Notes:

- `DifficultyOffset`: Difficulty = lerp(map min, map max difficulty, this); 0 = difficulty 1.0. Ignored when OverrideOfficialDifficulty > 0.
- `OverrideOfficialDifficulty`: If > 0, replaces the difficulty value outright (max wild level = 30 x difficulty); 0 = use DifficultyOffset.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `bAllowFlyerSpeedLeveling` † | ✔ | bool = False | switch | Flyer speed leveling allowed | Flyer speed leveling not allowed |
| `bAllowSpeedLeveling` † | ✔ | bool = False | switch | Speed leveling allowed | Speed leveling not allowed |
| `bAllowUnlimitedRespecs` † | ✔ | bool = False | switch | Unlimited respecs allowed | Unlimited respecs not allowed |
| `AlphaKillXPMultiplier` † | ✔ | float = 1 | ↑ XP gained | More XP | Less XP |
| `bAutoUnlockAllEngrams` | ✗ | bool = False | switch | On: auto unlock all engrams | Off |
| `BossKillXPMultiplier` † | ✔ | float = 1 | ↑ XP gained | More XP | Less XP |
| `CaveKillXPMultiplier` † | ✔ | float = 1 | ↑ XP gained | More XP | Less XP |
| `CraftXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |
| `EngramEntryAutoUnlocks` |  | list = (empty) |  |  |  |
| `ExplorerNoteXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |
| `GenericXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |
| `HarvestXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |
| `KillXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |
| `LevelExperienceRampOverrides` |  | list = (empty) |  |  |  |
| `MaxDifficulty` † | ✔ | bool = False | switch | Difficulty forced to 5.0 (levels up to 150) | Normal difficulty rules |
| `bOnlyAllowSpecifiedEngrams` | ✗ | bool = False | switch | Only allow specified engrams | Not restricted |
| `OverrideEngramEntries` |  | list = (empty) |  |  |  |
| `OverrideMaxExperiencePointsDino` | ✗ | int = 0 | ↑ creature XP cap | Tames can earn more total XP | Lower cap; 0 = game default |
| `OverrideMaxExperiencePointsPlayer` | ✗ | int = 0 | ↑ player XP cap | Players can earn more total XP (higher max level reachable) | Lower XP cap; 0 = game default |
| `OverrideNamedEngramEntries` |  | list = (empty) |  |  |  |
| `OverridePlayerLevelEngramPoints` |  | list = (empty) |  |  |  |
| `PerLevelStatsMultiplier_DinoTamed` † | ✔ | float[12] = [0]=0.2, [8]=0.17; others 0 | ↑ stat gain per level | More stat per level point | Less stat per level point; 0 = 1.0 |
| `PerLevelStatsMultiplier_DinoTamed_Add` † | ✔ | float[12] = [0]=0.14, [8]=0.14; others 0 | ↑ stat gain per level | More stat per level point | Less stat per level point; 0 = 1.0 |
| `PerLevelStatsMultiplier_DinoTamed_Affinity` † | ✔ | float[12] = [0]=0.44, [8]=0.44; others 0 | ↑ stat gain per level | More stat per level point | Less stat per level point; 0 = 1.0 |
| `PerLevelStatsMultiplier_DinoWild` † | ✔ | float[12] = all 0 | ↑ stat gain per level | More stat per level point | Less stat per level point; 0 = 1.0 |
| `PerLevelStatsMultiplier_Player` † | ✔ | float[12] = all 0 | ↑ stat gain per level | More stat per level point | Less stat per level point; 0 = 1.0 |
| `PlayerBaseStatMultipliers` † | ✔ | float[12] = all 0 | ↑ base stat | Higher base value for that stat (index = stat); 0 = unchanged | Lower base value |
| `SpecialXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |
| `TamedKillXPMultiplier` † | ✔ | float = 1 | ↑ XP gained | More XP | Less XP |
| `UnclaimedKillXPMultiplier` | ✔ × | float = 1 | ↑ XP gained | More XP | Less XP |
| `bUseDinoLevelUpAnimations` † | ✔ | bool = True | switch | Use dino level up animations | Don't use dino level up animations |
| `bUseSingleplayerSettings` † | ✔ | bool = False | switch | Single-player balance (tamed level-ups scaled up) | Normal balance |
| `WildKillXPMultiplier` † | ✔ | float = 1 | ↑ XP gained | More XP | Less XP |

Notes:

- `bAllowFlyerSpeedLeveling`: A flyer's 'can level speed' flag is set only when this is on.
- `bAllowSpeedLeveling`: Guards levelling the movement-speed stat (index 9); off = speed cannot be levelled (players).
- `bAllowUnlimitedRespecs`: When off, each respec records a real-time timestamp (respec cooldown); on = no cooldown.
- `AlphaKillXPMultiplier`: Selected by kill type, multiplied with KillXPMultiplier and base XP.
- `BossKillXPMultiplier`: Selected by kill type, multiplied with KillXPMultiplier and base XP.
- `CaveKillXPMultiplier`: Selected by kill type, multiplied with KillXPMultiplier and base XP.
- `MaxDifficulty`: When on, the difficulty value is forced to 5.0 (wild levels up to 150).
- `PerLevelStatsMultiplier_DinoTamed`: Per-stat level-up multiplier; 0 is treated as 1.0.
- `PerLevelStatsMultiplier_DinoTamed_Add`: Per-stat level-up multiplier; 0 is treated as 1.0.
- `PerLevelStatsMultiplier_DinoTamed_Affinity`: Per-stat level-up multiplier; 0 is treated as 1.0.
- `PerLevelStatsMultiplier_DinoWild`: Per-stat level-up multiplier; 0 is treated as 1.0.
- `PerLevelStatsMultiplier_Player`: Per-stat level-up multiplier; 0 is treated as 1.0.
- `PlayerBaseStatMultipliers`: Per stat: if > 0, the player's base stat value is multiplied by this.
- `TamedKillXPMultiplier`: Selected by kill type, multiplied with KillXPMultiplier and base XP.
- `bUseDinoLevelUpAnimations`: When on, creatures play the level-up animation.
- `bUseSingleplayerSettings`: Single-player balance; also scales the tamed per-level defaults (×2.125 health, ×2.35 melee damage, ×3.57 for the _Add values, ×2.27 for the _Affinity values).
- `WildKillXPMultiplier`: Selected by kill type, multiplied with KillXPMultiplier and base XP.

### PvP / PvE rules

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `AllowCaveBuildingPvE` † | ✔ | bool = False |  | switch | PvE: building in caves allowed | PvE: cave building refused |
| `AllowCaveBuildingPvP` † | ✔ | bool = True |  | switch | PvP: building in caves allowed | PvP: cave building refused |
| `AllowFlyerCarryPvE` † | ✔ | bool = False |  | switch | Flyer carry PvE allowed | Flyer carry PvE not allowed |
| `AllowRaidDinoFeeding` † | ✔ | bool = False |  | switch | Raid dino feeding allowed | Raid dino feeding not allowed |
| `AllowTeslaCoilCaveBuildingPVP` † | ✔ | bool = False |  | switch | Tesla coil cave building PvP allowed | Tesla coil cave building PvP not allowed |
| `DisableDinoDecayPvE` † | ✔ | bool = False |  | switch | PvE: tames never auto-decay | PvE: tames auto-decay |
| `DisablePvEGamma` | ✗ | bool = False |  | switch | PvE gamma off | PvE gamma on |
| `DisableStructureDecayPvE` † | ✔ | bool = False |  | switch | PvE: structures never auto-decay | PvE: structures auto-decay |
| `EnablePvPGamma` | ✗ | bool = False |  | switch | PvP gamma on | PvP gamma off |
| `IgnorePVPMountedWeaponryRestrictions` † | ◐ | bool = False |  | switch | On: ignore PvP mounted weaponry restrictions | Off |
| `PreventOfflinePvP` | ✗ | bool = False |  | switch | Offline PvP blocked | Offline PvP allowed |
| `PreventOfflinePvPInterval` | ✗ | float = 0 |  | ↑ prevent offline PvP interval | Longer | Shorter |
| `PvEAllowStructuresAtSupplyDrops` | ✗ | bool = False |  | switch | On: PvE allow structures at supply drops | Off |
| `PvEDinoDecayPeriodMultiplier` † | ✔ | float = 1 | yes | ↑ time before decay | Tames take longer to auto-decay | Tames decay sooner |
| `PvEStructureDecayPeriodMultiplier` † | ✔ | float = 1 |  | ↑ time before decay | Structures take longer to auto-decay | Structures decay sooner |
| `PvPDinoDecay` † | ✔ | bool = False |  | switch | PvP: tames auto-decay | PvP: no tame decay |
| `PvPStructureDecay` † | ✔ | bool = False |  | switch | PvP: structures auto-decay | PvP: no structure decay |
| `RaidDinoCharacterFoodDrainMultiplier` † | ✔ | float = 1 | yes | ↑ drain rate | Drains faster | Drains slower |
| `ServerHardcore` † | ✔ | bool = False |  | switch | Hardcore: death resets the survivor | Normal respawn |
| `ServerPVE` † | ✔ | bool = False |  | switch | PvE server | PvP server |

Notes:

- `AllowCaveBuildingPvE`: PvE servers: when off, structures can't be placed inside caves.
- `AllowCaveBuildingPvP`: PvP servers: when off, structures can't be placed inside caves.
- `AllowFlyerCarryPvE`: When on, flyers may pick up tribe-owned targets on PvE.
- `AllowRaidDinoFeeding`: Titanosaur / its inventory and Sweet Veggie Cake: when on, raid creatures can be fed.
- `AllowTeslaCoilCaveBuildingPVP`: When off, refused in caves on PvP.
- `DisableDinoDecayPvE`: PvE only: when on, creature decay timers are reset, so tames never auto-decay.
- `DisableStructureDecayPvE`: PvE only: when on, structure decay timers are reset, so structures never auto-decay.
- `IgnorePVPMountedWeaponryRestrictions`: On = PvP mounted-weapon restrictions ignored.
- `PvEDinoDecayPeriodMultiplier`: Creature decay period × this.
- `PvEStructureDecayPeriodMultiplier`: Structure decay period × this.
- `PvPDinoDecay`: PvP only: tames auto-decay only when this is on.
- `PvPStructureDecay`: PvP only: structures auto-decay only when this is on.
- `RaidDinoCharacterFoodDrainMultiplier`: Food drain of raid creatures x this.
- `ServerHardcore`: When on, hardcore death handling runs.
- `ServerPVE`: When on, damage between different tribes/players is blocked (PvE).

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `bAllowTeslaCoilCaveBuildingPVP` (also GUS) † | ✔ | bool = False | switch | Tesla coil cave building PvP allowed | Tesla coil cave building PvP not allowed |
| `AutoPvEStartTimeSeconds` | ✔ lim | float = 0 | ↑ auto PvE start time seconds | Longer | Shorter |
| `AutoPvEStopTimeSeconds` | ✔ lim | float = 0 | ↑ auto PvE stop time seconds | Longer | Shorter |
| `bAutoPvETimer` † | ✔ | bool = False | switch | On: auto PvE timer | Off |
| `bAutoPvEUseSystemTime` † | ✔ | bool = False | switch | On: auto PvE use system time | Off |
| `bDisableFriendlyFire` † | ✔ | bool = False | switch | Friendly fire off | Friendly fire on |
| `bDisableRailgunPVP` † | ✔ | bool = False | switch | Railgun PvP off | Railgun PvP on |
| `DisableRailgunPVP` † | ✔ | bool = False | switch | Railgun PvP off | Railgun PvP on |
| `bIncreasePvPRespawnInterval` † | ✔ | bool = True | switch | On: increase PvP respawn interval | Off |
| `IncreasePvPRespawnIntervalBaseAmount` † | ✔ | float = 60 | ↑ first PvP respawn penalty | Longer first respawn penalty | Shorter |
| `IncreasePvPRespawnIntervalCheckPeriod` | ✔ lim | float = 300 | ↑ increase PvP respawn interval check period | Longer | Shorter |
| `IncreasePvPRespawnIntervalMultiplier` | ✔ × | float = 2 | ↑ penalty growth | Respawn penalty grows faster with repeat deaths | Grows slower |
| `MaxAllowedRespawnInterval` † | ✔ | float = 1800 | ↑ respawn penalty cap | Longer maximum respawn delay | Shorter |
| `PreventOfflinePvPConnectionInvincibleInterval` | ✗ | float = 5 | ↑ prevent offline PvP connection invincible interval | Longer | Shorter |
| `bPvEAllowTribeWar` † | ✔ | bool = True | switch | On: PvE allow tribe war | Off |
| `bPvEAllowTribeWarCancel` | ✗ | bool = False | switch | On: PvE allow tribe war cancel | Off |
| `bPvEDisableFriendlyFire` † | ✔ | bool = False | switch | On: PvE disable friendly fire | Off |
| `PvPZoneStructureDamageMultiplier` † | ✔ | float = 6 | ↑ damage dealt | More damage | Less damage |

Notes:

- `bAllowTeslaCoilCaveBuildingPVP`: When off, refused in caves on PvP.
- `bAutoPvETimer`: When on, the server switches between PvE and PvP by time of day (in-game, or real time with bAutoPvEUseSystemTime), using AutoPvEStartTimeSeconds and AutoPvEStopTimeSeconds.
- `bAutoPvEUseSystemTime`: When on, the auto-PvE window uses the server's real clock; off = in-game time.
- `bDisableFriendlyFire`: When on, friendly fire is blocked.
- `bDisableRailgunPVP`: When on, it can't fire on PvP.
- `DisableRailgunPVP`: When on, it can't fire on PvP.
- `bIncreasePvPRespawnInterval`: Switches on the PvP respawn penalty that IncreasePvPRespawnInterval* configure.
- `IncreasePvPRespawnIntervalBaseAmount`: First PvP respawn penalty (seconds); later ones scale by IncreasePvPRespawnIntervalMultiplier, capped at MaxAllowedRespawnInterval.
- `MaxAllowedRespawnInterval`: Cap on the PvP respawn delay (seconds).
- `bPvEAllowTribeWar`: Off = no tribe wars on PvE servers.
- `bPvEDisableFriendlyFire`: When on, members of the same tribe deal no damage to each other (PvE).
- `PvPZoneStructureDamageMultiplier`: If > 0, damage to structures in PvP zones x this.

### Decay & auto-destroy

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `AutoDestroyDecayedDinos` † | ✔ | bool = False |  | switch | On: auto destroy decayed dinos | Off |
| `AutoDestroyOldStructuresMultiplier` † | ✔ | float = 0 |  | ↑ time before auto-destroy | Old structures are destroyed later | Destroyed sooner |
| `DisableBurrowDecayTimers` † | ✔ | bool = False |  | switch | Burrow decay timers off | Burrow decay timers on |
| `FastDecayUnsnappedCoreStructures` † | ✔ | bool = False |  | switch | On: fast decay unsnapped core structures | Off |
| `OnlyAutoDestroyCoreStructures` † | ✔ | bool = False |  | switch | Only auto destroy core structures | Not restricted |
| `OnlyDecayUnsnappedCoreStructures` † | ✔ | bool = False |  | switch | Only decay unsnapped core structures | Not restricted |
| `PreventPlannedStructureDecayReset` † | ✗ | bool = False |  | switch | Planned structure decay reset blocked | Planned structure decay reset allowed |

Notes:

- `AutoDestroyDecayedDinos`: When on, creatures that have decayed (claimable) are destroyed automatically.
- `AutoDestroyOldStructuresMultiplier`: Used only if > 0 (otherwise default); scales the auto-destroy period.
- `DisableBurrowDecayTimers`: Jackalope burrow structures: when on, their decay timers are skipped.
- `FastDecayUnsnappedCoreStructures`: When on, unsnapped core structures decay after FastDecayInterval.
- `OnlyAutoDestroyCoreStructures`: When on, auto-destroy only targets core structures (foundations, pillars).
- `OnlyDecayUnsnappedCoreStructures`: When on, only unsnapped core structures (foundations, pillars) auto-decay.
- `PreventPlannedStructureDecayReset`: Load not confirmed.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `CropDecaySpeedMultiplier` | ✔ × | float = 1 | ↑ crop decay speed | Crops rot faster | Crops rot slower |
| `bDisableBurrowDecayTimers` (also GUS) † | ✔ | bool = False | switch | Burrow decay timers off | Burrow decay timers on |
| `bDisableDinoDecayClaiming` † | ✔ | bool = False | switch | Dino decay claiming off | Dino decay claiming on |
| `FastDecayInterval` † | ✔ | float = 43200 | ↑ fast decay interval | Longer | Shorter |

Notes:

- `bDisableBurrowDecayTimers`: Jackalope burrow structures: when on, their decay timers are skipped.
- `bDisableDinoDecayClaiming`: When on, decayed creatures can't be claimed.
- `FastDecayInterval`: Seconds; decay time becomes min(this, normal decay) for fast-decaying structures.

### Structures & building

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `AllowBunkerModulesAboveGround` † | ✔ | bool = False |  | switch | Bunker modules above ground allowed | Bunker modules above ground not allowed |
| `AllowBunkerModulesInPreventionZones` † | ✔ | bool = False |  | switch | Bunker modules in prevention zones allowed | Bunker modules in prevention zones not allowed |
| `AllowBunkersInPreventionZones` † | ✔ | bool = False |  | switch | Bunkers in prevention zones allowed | Bunkers in prevention zones not allowed |
| `AllowCrateSpawnsOnTopOfStructures` † | ✔ | bool = False |  | switch | Crate spawns on top of structures allowed | Crate spawns on top of structures not allowed |
| `AllowDeprecatedStructures` † | ✗ | bool = False |  | switch | Deprecated structures allowed | Deprecated structures not allowed |
| `AllowDinoAIInsideBunkers` † | ✔ | bool = True | yes | switch | Dino AI inside bunkers allowed | Dino AI inside bunkers not allowed |
| `AllowMultipleAttachedC4` † | ✔ | bool = False |  | switch | Multiple attached c 4 allowed | Multiple attached c 4 not allowed |
| `AllowRidingDinosInsideBunkers` † | ✔ | bool = True | yes | switch | Riding dinos inside bunkers allowed | Riding dinos inside bunkers not allowed |
| `AlwaysAllowStructurePickup` † | ✔ | bool = True | yes | switch | On: always allow structure pickup | Off |
| `AnchoredVesselCheckRadius` † | ✗ | int = 30000 |  | ↑ anchored vessel check radius | Larger area | Smaller area |
| `BunkerUnderHPThresholdDmgMultiplier` † | ✔ | float = 0.05 | yes | ↑ damage to weakened bunkers | Bunkers under the threshold take more damage | Take less damage |
| `DestroyUnconnectedWaterPipes` † | ✔ | bool = False |  | switch | On: destroy unconnected water pipes | Off |
| `EnableExtraStructurePreventionVolumes` † | ✔ | bool = False |  | switch | Extra structure prevention volumes on | Extra structure prevention volumes off |
| `EnableMeshBitingProtection` † | ✔ | bool = True |  | switch | Mesh biting protection on | Mesh biting protection off |
| `EnableStructureComponentMemOpt` | ✗ | bool = False |  | switch | Structure component mem opt on | Structure component mem opt off |
| `EnemyAccessBunkerHPThreshold` † | ✔ | float = 0.25 | yes | ↑ HP fraction for enemy access | Enemies can enter bunkers at higher remaining HP | Bunker must be more damaged first |
| `ForceAllowCaveFlyers` † | ✔ | bool = False |  | switch | Allow cave flyers forced | Not forced |
| `ForceAllStructureLocking` † | ✔ | bool = False |  | switch | All structure locking forced | Not forced |
| `ForceGachaUnhappyInCaves` † | ✔ | bool = True | yes | switch | Gacha unhappy in caves forced | Not forced |
| `IgnoreLimitMaxStructuresInRangeTypeFlag` † | ✔ | bool = False |  | switch | On: ignore limit max structures in range type flag | Off |
| `LimitBunkersPerTribe` † | ✔ | bool = True | yes | switch | Bunkers per tribe limit on | No bunkers per tribe limit |
| `LimitBunkersPerTribeNum` † | ✔ | int = 3 | yes | ↑ limit bunkers per tribe num | Higher limit | Lower limit |
| `MaxAnchoredVesselsInRange` † | ✗ | int = 0 |  | ↑ max anchored vessels in range | Larger area | Smaller area |
| `MaxGateFrameOnSaddles` † | ✔ | int = -1 |  | ↑ max gate frame on saddles | Higher limit | Lower limit |
| `MaxStructuresInSmallRadius` † | ✔ | float = 0 |  | ↑ max structures in small radius | Larger area | Smaller area |
| `MinDistanceBetweenBunkers` † | ✔ | float = 3000 | yes | ↑ min distance between bunkers | Larger area | Smaller area |
| `NeedsPowerToActivateAquaticCompartments` † | ✔ | bool = True | yes | switch | On: needs power to activate aquatic compartments | Off |
| `OverrideStructurePlatformPrevention` † | ✔ | bool = False |  | switch | On: override structure platform prevention | Off |
| `PerPlatformMaxStructuresMultiplier` † | ✔ | float = 1 | yes | ↑ platform saddle cap | More structures on platform saddles | Fewer |
| `PlatformSaddleBuildAreaBoundsMultiplier` † | ◐ | float = 1 | yes | ↑ platform saddle build area bounds multiplier | Larger area | Smaller area |
| `PreventStructurePainting` † | ✔ | bool = False |  | switch | Structure painting blocked | Structure painting allowed |
| `PreventTemplateOnSaddle` † | ✔ | bool = False |  | switch | Template on saddle blocked | Template on saddle allowed |
| `RadiusStructuresInSmallRadius` † | ✔ | float = 0 |  | ↑ radius structures in small radius | Larger area | Smaller area |
| `ServerEnableMeshChecking` † | ✗ | bool = False |  | switch | On: server enable mesh checking | Off |
| `StructureDamageMultiplier` † | ✔ | float = 1 |  | ↑ damage dealt | More damage | Less damage |
| `StructurePickupHoldDuration` | ✗ | float = 0.5 | yes | ↑ hold time to pick up | Must hold the pickup key longer | Shorter hold |
| `StructurePickupTimeAfterPlacement` | ✗ | float = 30 | yes | ↑ pickup window | Structures can be picked up longer after placing | Shorter window |
| `StructureResistanceMultiplier` | ✔ × | float = 1 |  | ↑ damage structures take | Structures take more damage | Structures take less damage (tougher) |
| `TheMaxStructuresInRange` † | ◐ | float = 10500 | yes | ↑ build limit | More structures allowed in range (×1.633 in effect) | Fewer structures allowed |
| `TribeLogDestroyedEnemyStructures` † | ✔ | bool = False |  | switch | On: tribe log destroyed enemy structures | Off |

Notes:

- `AllowBunkerModulesAboveGround`: When on, bunker modules may be placed above ground.
- `AllowBunkerModulesInPreventionZones`: When off, bunker modules are refused inside prevention zones.
- `AllowBunkersInPreventionZones`: When off, bunkers are refused inside prevention zones.
- `AllowCrateSpawnsOnTopOfStructures`: When on, crates may land on structures.
- `AllowDeprecatedStructures`: Load not confirmed.
- `AllowDinoAIInsideBunkers`: When off, creature AI stops inside bunkers.
- `AllowMultipleAttachedC4`: When off, C4 can't be stuck onto a creature/player that already has one.
- `AllowRidingDinosInsideBunkers`: Whether creatures can be ridden inside bunkers.
- `AlwaysAllowStructurePickup`: When on, the pickup check skips the 'recently placed' window (StructurePickupTimeAfterPlacement): structures can always be picked up.
- `AnchoredVesselCheckRadius`: Values below 5000 are raised to 5000.
- `BunkerUnderHPThresholdDmgMultiplier`: Incoming damage × this once the bunker is under EnemyAccessBunkerHPThreshold.
- `DestroyUnconnectedWaterPipes`: When on, unconnected water pipes are destroyed after their timer.
- `EnableExtraStructurePreventionVolumes`: When on, extra structure-prevention volumes are applied.
- `EnableMeshBitingProtection`: When on, creature bites through meshes are blocked. Setting it to False has no effect; the server keeps it on.
- `EnemyAccessBunkerHPThreshold`: Enemies can access when health / max health ≤ this (fraction 0–1).
- `ForceAllowCaveFlyers`: When on, flyers are allowed in caves.
- `ForceAllStructureLocking`: When on, the lock flags are set on all structures.
- `ForceGachaUnhappyInCaves`: When on, gachas are unhappy in caves.
- `IgnoreLimitMaxStructuresInRangeTypeFlag`: When on, the build-limit count skips the per-structure-type exemption check.
- `LimitBunkersPerTribe`: When on, LimitBunkersPerTribeNum applies.
- `LimitBunkersPerTribeNum`: Max bunkers per tribe (with LimitBunkersPerTribe).
- `MaxAnchoredVesselsInRange`: Load not confirmed.
- `MaxGateFrameOnSaddles`: Negative = no limit; 0 = none allowed; otherwise max gateways per platform saddle.
- `MaxStructuresInSmallRadius`: If > 0, max structures within RadiusStructuresInSmallRadius.
- `MinDistanceBetweenBunkers`: If > 0, bunkers must be at least this far apart.
- `NeedsPowerToActivateAquaticCompartments`: When on, aquatic compartments need power to activate.
- `OverrideStructurePlatformPrevention`: When on, restricted structures can be built on platform saddles.
- `PerPlatformMaxStructuresMultiplier`: Platform saddle structure cap x this.
- `PlatformSaddleBuildAreaBoundsMultiplier`: No map in the game sets this; its effect is not confirmed.
- `PreventStructurePainting`: When on, structures can't be painted.
- `PreventTemplateOnSaddle`: When on, structure templates can't be placed on saddles.
- `RadiusStructuresInSmallRadius`: Radius for MaxStructuresInSmallRadius.
- `ServerEnableMeshChecking`: Load not confirmed.
- `StructureDamageMultiplier`: Damage dealt by structures (turrets, traps) x this.
- `TheMaxStructuresInRange`: The effective limit is this × 1.633 (default 10500 → 17146). Official servers force 10500.
- `TribeLogDestroyedEnemyStructures`: When on, destroying an enemy structure is written to the tribe log.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `bAllowBunkerModulesAboveGround` (also GUS) † | ✔ | bool = False | switch | Bunker modules above ground allowed | Bunker modules above ground not allowed |
| `bAllowBunkerModulesInPreventionZones` (also GUS) † | ✔ | bool = False | switch | Bunker modules in prevention zones allowed | Bunker modules in prevention zones not allowed |
| `bAllowBunkersInPreventionZones` (also GUS) † | ✔ | bool = False | switch | Bunkers in prevention zones allowed | Bunkers in prevention zones not allowed |
| `bAllowDinoAIInsideBunkers` (also GUS) † | ✔ | bool = True | switch | Dino AI inside bunkers allowed | Dino AI inside bunkers not allowed |
| `bAllowPlatformSaddleMultiFloors` † | ◐ | bool = False | switch | Platform saddle multi floors allowed | Platform saddle multi floors not allowed |
| `bAllowRidingDinosInsideBunkers` (also GUS) † | ✔ | bool = True | switch | Riding dinos inside bunkers allowed | Riding dinos inside bunkers not allowed |
| `BunkerUnderHPThresholdDmgMultiplier` (also GUS) † | ✔ | float = 0.05 | ↑ damage to weakened bunkers | Bunkers under the threshold take more damage | Take less damage |
| `CityOutpostAllowMaxStructuresNearby` † | ✔ | int = 5 | ↑ structures near city outposts | More structures allowed nearby | Fewer |
| `CropPlotStackLimit` † | ✔ | int = 100 | ↑ crop plot stack limit | Higher limit | Lower limit |
| `DinoTurretDamageMultiplier` | ✔ × | float = 1 | ↑ damage dealt | More damage | Less damage |
| `bDisableStructurePlacementCollision` † | ✔ | bool = False | switch | Structure placement collision off | Structure placement collision on |
| `bDisableWirelessCraftingForStructures` † | ✔ | bool = False | switch | Wireless crafting for structures off | Wireless crafting for structures on |
| `EnemyAccessBunkerHPThreshold` (also GUS) † | ✔ | float = 0.25 | ↑ HP fraction for enemy access | Enemies can enter bunkers at higher remaining HP | Bunker must be more damaged first |
| `bFlyerPlatformAllowUnalignedDinoBasing` † | ✔ | bool = True | switch | On: flyer platform allow unaligned dino basing | Off |
| `bForceAllowAllStructures` | ✗ | bool = False | switch | Allow all structures forced | Not forced |
| `bForceGachaUnhappyInCaves` (also GUS) † | ✔ | bool = True | switch | Gacha unhappy in caves forced | Not forced |
| `bGenesisUseStructuresPreventionVolumes` | ✗ | bool = False | switch | On: genesis use structures prevention volumes | Off |
| `bHardLimitTurretsInRange` † | ✔ | bool = False | switch | On: hard limit turrets in range | Off |
| `bIgnoreStructuresPreventionVolumes` † | ✔ | bool = False | switch | On: ignore structures prevention volumes | Off |
| `bLimitBunkersPerTribe` (also GUS) † | ✔ | bool = True | switch | Bunkers per tribe limit on | No bunkers per tribe limit |
| `LimitBunkersPerTribeNum` (also GUS) † | ✔ | int = 3 | ↑ limit bunkers per tribe num | Higher limit | Lower limit |
| `LimitCloningChambersInRangeNum` † | ✔ | int = 10 | ↑ limit cloning chambers in range num | Larger area | Smaller area |
| `LimitCloningChambersInTribeNum` † | ✔ | int = 50 | ↑ limit cloning chambers in tribe num | Higher limit | Lower limit |
| `LimitCloningChambersRange` † | ✔ | float = 10000 | ↑ limit cloning chambers range | Larger area | Smaller area |
| `LimitGeneratorsNum` † | ✔ | int = 3 | ↑ limit generators num | Higher limit | Lower limit |
| `LimitGeneratorsRange` † | ✔ | float = 15000 | ↑ limit generators range | Larger area | Smaller area |
| `LimitLinkedStoragesNum` † | ✔ | int = 4 | ↑ limit linked storages num | Higher limit | Lower limit |
| `LimitReplicatorsPerSaddle` † | ✔ | int = 1 | ↑ limit replicators per saddle | Higher limit | Lower limit |
| `bLimitTurretsInRange` † | ✔ | bool = True | switch | Turrets in range limit on | No turrets in range limit |
| `LimitTurretsNum` † | ✔ | int = 100 | ↑ limit turrets num | Higher limit | Lower limit |
| `LimitTurretsRange` † | ✔ | float = 10000 | ↑ limit turrets range | Larger area | Smaller area |
| `MaxGateFrameOnSaddles` (also GUS) † | ✔ | int = -1 | ↑ max gate frame on saddles | Higher limit | Lower limit |
| `MaxPerTribePlatformSaddleStructureLimit` | ✗ | int = 0 | ↑ max per tribe platform saddle structure limit | Higher limit | Lower limit |
| `MaxPlatformSaddleStructureLimit` † | ✔ | int = 75 | ↑ max platform saddle structure limit | Higher limit | Lower limit |
| `MaxStructuresToAllowForPickupOverride` | ✗ | int = -1 | ↑ max structures to allow for pickup override | Higher limit | Lower limit |
| `MeshCheckingPercentageToFail` † | ✔ | float = 0.2 | ↑ failure threshold | Anti-mesh check is more tolerant | Stricter |
| `MeshCheckingRayDistance` † | ✔ | float = 60000 | ↑ trace distance | Anti-mesh traces reach farther | Shorter traces |
| `MeshCheckingSubdivisons` † | ✔ | float = 10 | ↑ trace count | More thorough anti-mesh check (more CPU) | Coarser check |
| `MinDistanceBetweenBunkers` (also GUS) † | ✔ | float = 3000 | ↑ min distance between bunkers | Larger area | Smaller area |
| `bNeedsPowerToActivateAquaticCompartments` (also GUS) † | ✔ | bool = True | switch | On: needs power to activate aquatic compartments | Off |
| `OutpostAllowMaxStructuresNearby` † | ✔ | int = 50 | ↑ structures near outposts | More structures allowed nearby | Fewer |
| `bPassiveDefensesDamageRiderlessDinos` † | ✔ | bool = False | switch | On: passive defenses damage riderless dinos | Off |
| `bPreventTemplateOnSaddle` (also GUS) † | ✔ | bool = False | switch | Template on saddle blocked | Template on saddle allowed |
| `RTSProximityToEnemyStructureScale` | ✗ | float = 1 | ↑ RTS proximity to enemy structure scale | More / stronger | Less / weaker |
| `StructureDamageRepairCooldown` | ✔ lim | float = 180 | ↑ structure damage repair cooldown | Longer | Shorter |
| `UseStructurePreventionVolumeTagString` |  | string = — |  |  |  |

Notes:

- `bAllowBunkerModulesAboveGround`: When on, bunker modules may be placed above ground.
- `bAllowBunkerModulesInPreventionZones`: When off, bunker modules are refused inside prevention zones.
- `bAllowBunkersInPreventionZones`: When off, bunkers are refused inside prevention zones.
- `bAllowDinoAIInsideBunkers`: When off, creature AI stops inside bunkers.
- `bAllowPlatformSaddleMultiFloors`: No map in the game sets this; its effect is not confirmed.
- `bAllowRidingDinosInsideBunkers`: Whether creatures can be ridden inside bunkers.
- `BunkerUnderHPThresholdDmgMultiplier`: Incoming damage × this once the bunker is under EnemyAccessBunkerHPThreshold.
- `CityOutpostAllowMaxStructuresNearby`: Lost Colony city outposts: max player structures near a spawn point (above this, the outpost doesn't spawn there).
- `CropPlotStackLimit`: Max crop plots stacked on each other (shown as "Can't stack more than N crop plots").
- `bDisableStructurePlacementCollision`: When on, structure placement skips the world-geometry obstruction check.
- `bDisableWirelessCraftingForStructures`: When on, structures can't craft wirelessly.
- `EnemyAccessBunkerHPThreshold`: Enemies can access when health / max health ≤ this (fraction 0–1).
- `bFlyerPlatformAllowUnalignedDinoBasing`: When on, creatures may stand unaligned on flyer platform saddles.
- `bForceGachaUnhappyInCaves`: When on, gachas are unhappy in caves.
- `bHardLimitTurretsInRange`: Turret hard-limit checks (turrets, Spindles): when on, turrets over the limit stop working.
- `bIgnoreStructuresPreventionVolumes`: When on, structure-prevention volumes are ignored. Not applied when set in Game.ini.
- `bLimitBunkersPerTribe`: When on, LimitBunkersPerTribeNum applies.
- `LimitBunkersPerTribeNum`: Max bunkers per tribe (with LimitBunkersPerTribe).
- `LimitCloningChambersInRangeNum`: If ≥ 0, max chambers within LimitCloningChambersRange.
- `LimitCloningChambersInTribeNum`: If ≥ 0, max chambers per tribe.
- `LimitCloningChambersRange`: Radius for LimitCloningChambersInRangeNum.
- `LimitGeneratorsNum`: If > 0, max generators within LimitGeneratorsRange.
- `LimitGeneratorsRange`: If > 0, radius for LimitGeneratorsNum.
- `LimitLinkedStoragesNum`: Linked storage: if > 0, max linked storages.
- `LimitReplicatorsPerSaddle`: Tek Replicator: if ≥ 0, max replicators per saddle.
- `bLimitTurretsInRange`: When on, LimitTurretsNum / LimitTurretsRange apply.
- `LimitTurretsNum`: Max turrets within LimitTurretsRange (with bLimitTurretsInRange).
- `LimitTurretsRange`: Radius for the turret limit (with bLimitTurretsInRange).
- `MaxGateFrameOnSaddles`: Negative = no limit; 0 = none allowed; otherwise max gateways per platform saddle.
- `MaxPlatformSaddleStructureLimit`: If > 0, max structures on a platform saddle.
- `MeshCheckingPercentageToFail`: Parameter of the anti-meshing check.
- `MeshCheckingRayDistance`: Parameter of the anti-meshing check (trace distance).
- `MeshCheckingSubdivisons`: Parameter of the anti-meshing check (trace count).
- `MinDistanceBetweenBunkers`: If > 0, bunkers must be at least this far apart.
- `bNeedsPowerToActivateAquaticCompartments`: When on, aquatic compartments need power to activate.
- `OutpostAllowMaxStructuresNearby`: Lost Colony outposts: max player structures near a spawn point (above this, the outpost doesn't spawn there).
- `bPassiveDefensesDamageRiderlessDinos`: Spike walls: when on, they also damage riderless creatures.
- `bPreventTemplateOnSaddle`: When on, structure templates can't be placed on saddles.

### Harvesting & resources

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `BloodforgeReinforceResourceCostMultiplier` | ✗ | float = 3 | yes | ↑ reinforcing cost | Reinforcing costs more | Costs less |
| `ClampResourceHarvestDamage` † | ✔ | bool = False |  | switch | On: clamp resource harvest damage | Off |
| `HarvestAmountMultiplier` † | ✔ | float = 1 |  | ↑ resources per hit | More resources per hit | Fewer resources per hit |
| `HarvestHealthMultiplier` | ✔ ÷ | float = 1 |  | ↑ resource node HP | Nodes take more hits (more total yield per node) | Nodes break sooner |
| `ResourcesRespawnPeriodMultiplier` † | ✔ | float = 1 |  | ↑ respawn time | Resources respawn slower | Resources respawn faster |
| `StructurePreventResourceRadiusMultiplier` | ✔ × | float = 1 | yes | ↑ no-respawn radius | Structures block resource respawn over a wider area | Smaller blocked area |
| `UseOptimizedHarvestingHealth` † | ✔ | bool = False |  | switch | Use optimized harvesting health | Don't use optimized harvesting health |

Notes:

- `ClampResourceHarvestDamage`: When on, damage dealt to a resource node is clamped to the node's remaining health.
- `HarvestAmountMultiplier`: Resources per hit = count x this (a per-character modifier can override).
- `ResourcesRespawnPeriodMultiplier`: Node min/max respawn time x this.
- `UseOptimizedHarvestingHealth`: When on, harvest yield comes from accumulated damage: every (per-item health ÷ HarvestHealthMultiplier) of damage gives one batch, remainder carried over.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `BloodforgeReinforceResourceCostMultiplier` (also GUS) | ✗ | float = 3 | ↑ reinforcing cost | Reinforcing costs more | Costs less |
| `CropGrowthSpeedMultiplier` | ✗ | float = 1 | ↑ crop growth speed | Crops grow faster | Crops grow slower |
| `DinoHarvestingDamageMultiplier` | ✔ × | float = 3.2 | ↑ damage dealt | More damage | Less damage |
| `FishingLootQualityMultiplier` † | ✔ | float = 1 | ↑ fishing loot quality | More / higher | Less / lower |
| `HarvestResourceItemAmountClassMultipliers` |  | list = (empty) |  |  |  |
| `MaxActiveResourceCaches` † | ✔ | int = 8 | ↑ max active resource caches | Higher limit | Lower limit |
| `MaxActiveResourceCachesSP` † | ✔ | int = 8 | ↑ max active resource caches SP | Higher limit | Lower limit |
| `PlayerHarvestingDamageMultiplier` | ✔ × | float = 1 | ↑ damage dealt | More damage | Less damage |
| `ResourceNoReplenishRadiusPlayers` | ✗ | float = 1 | ↑ resource no replenish radius players | Larger area | Smaller area |
| `ResourceNoReplenishRadiusStructures` | ✔ × | float = 1 | ↑ resource no replenish radius structures | Larger area | Smaller area |

Notes:

- `FishingLootQualityMultiplier`: Fishing rod / net: loot quality × this; 0 is special-cased.
- `MaxActiveResourceCaches`: Lost Colony resource caches: maximum active at once = this.
- `MaxActiveResourceCachesSP`: Lost Colony resource caches (single player): maximum active at once = this.

### Items, crafting & loot

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `BloodforgeReinforceExtraDurability` † | ✔ | float = 0.3 | yes | ↑ durability from reinforcing | Reinforced items gain more durability | Less durability gain |
| `BloodforgeReinforceSpeedMultiplier` † | ✔ | float = 0.1 | yes | ↑ reinforcing speed | Reinforcing is faster | Slower |
| `ClampItemSpoilingTimes` † | ✔ | bool = False |  | switch | On: clamp item spoiling times | Off |
| `ClampItemStats` † | ✔ | bool = False |  | switch | On: clamp item stats | Off |
| `CosmoWeaponAmmoReloadAmount` † | ✔ | float = 0 |  | ↑ ammo per reload | More ammo restored per reload | Less |
| `DinoStackingCheckRadius` † | ✔ | float = 50 |  | ↑ dino stacking check radius | Larger area | Smaller area |
| `DinoStackingChecksPerFrame` | ✗ | int = 5 |  | ↑ stacking checks per frame | Stacking detection catches up faster (more CPU) | Fewer checks per frame |
| `DinoStackingMaxOverlap` | ✗ | int = 50 |  | ↑ allowed creature overlap | More stacked creatures tolerated | Fewer tolerated |
| `DisableDinoItemBlacklist` | ✗ | bool = False |  | switch | Dino item blacklist off | Dino item blacklist on |
| `EnableDinoStackingDetection` | ✗ | bool = False |  | switch | Dino stacking detection on | Dino stacking detection off |
| `ItemStackSizeMultiplier` † | ✔ | float = 1 | yes | ↑ max stack size | Bigger stacks | Smaller stacks (never below 1) |
| `MaxBlueprintDinoLevel` | ✗ | int = 0 |  | ↑ max blueprint dino level | Higher limit | Lower limit |
| `MaxBlueprintDinoQuality` | ✗ | int = 0 |  | ↑ max blueprint dino quality | Higher limit | Lower limit |
| `MaxBlueprintItemQuality` | ✗ | int = 0 |  | ↑ max blueprint item quality | Higher limit | Lower limit |
| `MaxBlueprintScoutQuality` | ✗ | int = 0 |  | ↑ max blueprint scout quality | Higher limit | Lower limit |
| `MaxCosmoWeaponAmmo` † | ✔ | float = 0 |  | ↑ max ammo | Higher ammo cap | Lower cap |
| `MaxTributeItems` † | ✔ | int = 50 |  | ↑ max tribute items | Higher limit | Lower limit |
| `OnLoadItemQualityRatingClamp` † | ✔ | float = 0 |  | ↑ on load item quality rating clamp | Higher limit | Lower limit |
| `PreventDownloadItems` † | ✔ | bool = False |  | switch | Download items blocked | Download items allowed |
| `PreventDroppedItemPhysics` † | ✔ | bool = True |  | switch | Dropped item physics blocked | Dropped item physics allowed |
| `PreventItemTraitEffects` † | ✔ | bool = False |  | switch | Item trait effects blocked | Item trait effects allowed |
| `PreventNonClusterCharacterUploadsWithItems` † | ✔ | bool = False |  | switch | Non cluster character uploads with items blocked | Non cluster character uploads with items allowed |
| `PreventUploadItems` † | ✔ | bool = False |  | switch | Upload items blocked | Upload items allowed |
| `RandomSupplyCratePoints` † | ✔ | bool = False |  | switch | On: random supply crate points | Off |
| `TributeItemExpirationSeconds` † | ✔ | int = 86400 |  | ↑ tribute item expiration seconds | Longer | Shorter |
| `UseOnLoadClampItemQualityRating` † | ✔ | bool = False |  | switch | Use on load clamp item quality rating | Don't use on load clamp item quality rating |

Notes:

- `BloodforgeReinforceExtraDurability`: Reinforced items get (1 + this) × durability.
- `BloodforgeReinforceSpeedMultiplier`: Repair-speed multiplier while reinforcing at the Bloodforge.
- `ClampItemSpoilingTimes`: Switches on clamping item spoil times.
- `ClampItemStats`: Switches on clamping item stats to ItemStatClamps.
- `CosmoWeaponAmmoReloadAmount`: Cosmo: if > 0, web ammo per reload = min(this, clip size).
- `DinoStackingCheckRadius`: Radius of the creature-stacking check.
- `ItemStackSizeMultiplier`: Max stack = max(1, round(item max x this)) for items without a stack override and a max stack above 1.
- `MaxCosmoWeaponAmmo`: Cosmo: if > 0, max web ammo = min(this, clip size).
- `MaxTributeItems`: Passed to the upload-limit check for items.
- `OnLoadItemQualityRatingClamp`: If non-zero (with UseOnLoadClampItemQualityRating), items above this quality are clamped on load.
- `PreventDownloadItems`: When on, item downloads are refused.
- `PreventDroppedItemPhysics`: When on, dropped items don't simulate physics.
- `PreventItemTraitEffects`: When on, item traits have no effect.
- `PreventNonClusterCharacterUploadsWithItems`: When on, survivors carrying items can't be uploaded outside a cluster.
- `PreventUploadItems`: When on, item uploads are refused.
- `RandomSupplyCratePoints`: When on, supply crates use random spawn points; off = their fixed points.
- `TributeItemExpirationSeconds`: If > 0, uploaded items expire after this many seconds.
- `UseOnLoadClampItemQualityRating`: When on, item quality is clamped on load (to OnLoadItemQualityRatingClamp).

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `bAllowCustomRecipes` † | ✔ | bool = True | switch | Custom recipes allowed | Custom recipes not allowed |
| `BloodforgeReinforceExtraDurability` (also GUS) † | ✔ | float = 0.3 | ↑ durability from reinforcing | Reinforced items gain more durability | Less durability gain |
| `BloodforgeReinforceSpeedMultiplier` (also GUS) † | ✔ | float = 0.1 | ↑ reinforcing speed | Reinforcing is faster | Slower |
| `ConfigOverrideItemCraftingCosts` |  | list = (empty) |  |  |  |
| `ConfigOverrideItemMaxQuantity` |  | list = (empty) |  |  |  |
| `ConfigOverrideSupplyCrateItems` |  | list = (empty) |  |  |  |
| `ConfigSettingsModifiedTraitEffectiveness` |  | map = (empty) |  |  |  |
| `ConfigSettingsModifiedTraitEffectiveness_Key` |  | name = — |  |  |  |
| `CosmoWeaponAmmoReloadAmount` (also GUS) † | ✔ | int = 0 | ↑ ammo per reload | More ammo restored per reload | Less |
| `CraftingSkillBonusMultiplier` † | ✔ | float = 1 | ↑ crafting skill bonus | More / higher | Less / lower |
| `CustomRecipeEffectivenessMultiplier` | ✗ | float = 1 | ↑ custom recipe effectiveness | More / higher | Less / lower |
| `CustomRecipeSkillMultiplier` | ✗ | float = 1 | ↑ custom recipe skill | More / stronger | Less / weaker |
| `bDisableDefaultMapItemSets` † | ✔ | bool = False | switch | Default map item sets off | Default map item sets on |
| `bDisableDinoItemBlacklist` (also GUS) | ✗ | bool = False | switch | Dino item blacklist off | Dino item blacklist on |
| `bDisableLootCrates` † | ✔ | bool = False | switch | Loot crates off | Loot crates on |
| `bDisableWirelessCrafting` † | ✔ | bool = False | switch | Wireless crafting off | Wireless crafting on |
| `bDisableWirelessCraftingForDinos` † | ✔ | bool = False | switch | Wireless crafting for dinos off | Wireless crafting for dinos on |
| `bDisableWirelessCraftingForPlayers` † | ✔ | bool = False | switch | Wireless crafting for players off | Wireless crafting for players on |
| `ExcludeItemIndices` |  | list = (empty) |  |  |  |
| `bForceAllowAscensionItemDownloads` † | ✔ | bool = True | switch | Allow ascension item downloads forced | Not forced |
| `bForceUseInventoryAppends` † | ◐ | bool = False | switch | Use inventory appends forced | Not forced |
| `FuelConsumptionIntervalMultiplier` | ✔ × | float = 1 | ↑ fuel duration | Fuel lasts longer | Fuel burns faster |
| `GlobalCorpseDecompositionTimeMultiplier` † | ✔ | float = 0 | ↑ corpse lifetime | Corpses last longer | Corpses vanish sooner |
| `GlobalItemDecompositionTimeMultiplier` † | ✔ | float = 0 | ↑ dropped-item lifetime | Dropped items last longer | Dropped items vanish sooner |
| `GlobalPoweredBatteryDurabilityDecreasePerSecond` | ✔ × | float = 3 | ↑ battery drain | Batteries drain faster | Drain slower |
| `GlobalSpoilingTimeMultiplier` † | ✔ | float = 0 | ↑ spoil time | Items spoil slower | Items spoil faster |
| `ItemStackSizeMultiplier` (also GUS) † | ✔ | float = 1 | ↑ max stack size | Bigger stacks | Smaller stacks (never below 1) |
| `ItemStatClamps` † | ✔ | int[8] = all 0 | ↑ item stat clamps | Higher limit | Lower limit |
| `LimitNonPlayerDroppedItemsCount` † | ✔ | int = 0 | ↑ limit non player dropped items count | Higher limit | Lower limit |
| `LimitNonPlayerDroppedItemsRange` † | ✔ | float = 0 | ↑ limit non player dropped items range | Larger area | Smaller area |
| `MaxCosmoWeaponAmmo` (also GUS) † | ✔ | int = 0 | ↑ max ammo | Higher ammo cap | Lower cap |
| `MinimumTimebetweeninventoryRetrieval` | ✗ | float = 3600 | ↑ minimum timebetweeninventory retrieval | Higher limit | Lower limit |
| `OnLoadItemQualityRatingClamp` (also GUS) † | ✔ | float = 0 | ↑ on load item quality rating clamp | Higher limit | Lower limit |
| `bPreventItemTraitEffects` (also GUS) † | ✔ | bool = False | switch | Item trait effects blocked | Item trait effects allowed |
| `SingleplayerSettingsCorpseLifespanMultiplier` | ✗ | float = 6 | ↑ lifetime | Lasts longer | Disappears sooner |
| `SupplyCrateLootQualityMultiplier` | ✔ × | float = 1 | ↑ supply crate loot quality | More / higher | Less / lower |
| `UseCorpseLifeSpanMultiplier` † | ✔ | float = 1 | ↑ corpse lifetime | Corpses last longer | Corpses vanish sooner |
| `bUseOnLoadClampItemQualityRating` (also GUS) † | ✔ | bool = False | switch | Use on load clamp item quality rating | Don't use on load clamp item quality rating |
| `WirelessCraftingRangeOverride` | ✔ lim | float = 0 | ↑ wireless crafting range override | Larger area | Smaller area |

Notes:

- `bAllowCustomRecipes`: Custom-recipe items are only processed (recipe effects applied) when this is on.
- `BloodforgeReinforceExtraDurability`: Reinforced items get (1 + this) × durability.
- `BloodforgeReinforceSpeedMultiplier`: Repair-speed multiplier while reinforcing at the Bloodforge.
- `CosmoWeaponAmmoReloadAmount`: Cosmo: if > 0, web ammo per reload = min(this, clip size).
- `CraftingSkillBonusMultiplier`: Multiplies the crafting-skill stat bonus.
- `bDisableDefaultMapItemSets`: When on, the map's default item sets are not used.
- `bDisableLootCrates`: When on, crates are not spawned.
- `bDisableWirelessCrafting`: When on, wireless crafting (pulling resources from nearby storage) is off.
- `bDisableWirelessCraftingForDinos`: When on, creatures can't craft wirelessly.
- `bDisableWirelessCraftingForPlayers`: When on, players can't craft wirelessly.
- `bForceAllowAscensionItemDownloads`: When on, ascension items are processed in downloads (allowed).
- `bForceUseInventoryAppends`: When on, configured inventory additions are applied.
- `GlobalCorpseDecompositionTimeMultiplier`: Used only if > 0 (otherwise default); scales corpse lifetime.
- `GlobalItemDecompositionTimeMultiplier`: Used only if > 0 (otherwise default); scales dropped-item lifetime.
- `GlobalSpoilingTimeMultiplier`: Spoil time = item spoil time x this; used only if > 0.
- `ItemStackSizeMultiplier`: Max stack = max(1, round(item max x this)) for items without a stack override and a max stack above 1.
- `ItemStatClamps`: Per-stat item stat cap; 0 = no clamp.
- `LimitNonPlayerDroppedItemsCount`: Max non-player dropped items within the range.
- `LimitNonPlayerDroppedItemsRange`: Radius for counting non-player dropped items; 0 = off.
- `MaxCosmoWeaponAmmo`: Cosmo: if > 0, max web ammo = min(this, clip size).
- `OnLoadItemQualityRatingClamp`: If non-zero (with UseOnLoadClampItemQualityRating), items above this quality are clamped on load.
- `bPreventItemTraitEffects`: When on, item traits have no effect.
- `UseCorpseLifeSpanMultiplier`: Corpse lifetime = species lifetime x this.
- `bUseOnLoadClampItemQualityRating`: When on, item quality is clamped on load (to OnLoadItemQualityRatingClamp).

### Creatures, spawns & stats

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `AllowFlyingStaminaRecovery` † | ✔ | bool = False |  | switch | Flying stamina recovery allowed | Flying stamina recovery not allowed |
| `AllowTekSuitPowersInGenesis` † | ✔ | bool = False |  | switch | Tek suit powers in genesis allowed | Tek suit powers in genesis not allowed |
| `AllowVariantDinoPacks` † | ✔ | bool = False |  | switch | Variant dino packs allowed | Variant dino packs not allowed |
| `DinoCharacterFoodDrainMultiplier` † | ✔ | float = 1 |  | ↑ drain rate | Drains faster | Drains slower |
| `DinoCharacterHealthRecoveryMultiplier` † | ✔ | float = 1 |  | ↑ recovery speed | Recovers faster | Recovers slower |
| `DinoCharacterStaminaDrainMultiplier` † | ✔ | float = 1 |  | ↑ drain rate | Drains faster | Drains slower |
| `DinoCountMultiplier` † | ✔ | float = 1 |  | ↑ wild creature count | More wild creatures | Fewer wild creatures; ~0 disables some spawners |
| `DinoDamageMultiplier` † | ✔ | float = 1 |  | ↑ damage dealt | More damage | Less damage |
| `DinoResistanceMultiplier` | ✔ × | float = 1 |  | ↑ damage creatures take | Creatures take more damage | Creatures take less damage (tougher) |
| `DisableTekLegsBoost` † | ✔ | bool = False |  | switch | Tek legs boost off | Tek legs boost on |
| `EnablePlayerMoveThroughAlly` | ✗ | bool = False |  | switch | Player move through ally on | Player move through ally off |
| `EnablePlayerMoveThroughSleeping` † | ✔ | bool = False |  | switch | Player move through sleeping on | Player move through sleeping off |
| `bForceCanRideFliers` | ✗ | bool = False |  | switch | Can ride fliers forced | Not forced |
| `ForceResetWildDinos` † | ✔ | bool = False |  | switch | Reset wild dinos forced | Not forced |
| `NonPermanentDiseases` † | ✔ | bool = False |  | switch | No n permanent diseases | N permanent diseases allowed |
| `OxygenSwimSpeedStatMultiplier` | ✔ × | float = 1 | yes | ↑ swim speed from oxygen | Oxygen levels give more swim speed | Less swim speed |
| `PlayerCharacterFoodDrainMultiplier` | ✔ × | float = 1 |  | ↑ drain rate | Drains faster | Drains slower |
| `PlayerCharacterHealthRecoveryMultiplier` | ✔ × | float = 1 |  | ↑ recovery speed | Recovers faster | Recovers slower |
| `PlayerCharacterStaminaDrainMultiplier` † | ✔ | float = 1 |  | ↑ drain rate | Drains faster | Drains slower |
| `PlayerCharacterWaterDrainMultiplier` | ✔ × | float = 1 |  | ↑ drain rate | Drains faster | Drains slower |
| `PlayerDamageMultiplier` † | ✔ | float = 1 |  | ↑ damage dealt | More damage | Less damage |
| `PlayerResistanceMultiplier` | ✔ × | float = 1 |  | ↑ damage players take | Players take more damage | Players take less damage (tougher) |
| `PreventDiseases` † | ✔ | bool = False |  | switch | Diseases blocked | Diseases allowed |
| `PreventOverworldBosses` † | ✔ | bool = False |  | switch | Overworld bosses blocked | Overworld bosses allowed |
| `PreventSpawnAnimations` † | ✔ | bool = False |  | switch | Spawn animations blocked | Spawn animations allowed |
| `RiderDinoCollision` † | ✔ | bool = False |  | switch | On: rider dino collision | Off |
| `ServerAutoForceRespawnWildDinosInterval` | ✗ | float = 0 |  | ↑ server auto force respawn wild dinos interval | Longer | Shorter |

Notes:

- `AllowFlyingStaminaRecovery`: When on, flyers recover stamina while flying.
- `AllowTekSuitPowersInGenesis`: When off, Tek suit powers are restricted in Genesis.
- `AllowVariantDinoPacks`: When on, variant creature pack entries are used when spawning.
- `DinoCharacterFoodDrainMultiplier`: Creature food drain x this.
- `DinoCharacterHealthRecoveryMultiplier`: Creature health recovery x this.
- `DinoCharacterStaminaDrainMultiplier`: Creature stamina drain x this.
- `DinoCountMultiplier`: Clamped and used in the wild spawn-count formula; ~0 also switches a spawner flag off. Load not confirmed.
- `DinoDamageMultiplier`: In this build it only affects the crab's attack; it has no effect on other creatures.
- `DisableTekLegsBoost`: When on, the Tek run boost is blocked.
- `EnablePlayerMoveThroughSleeping`: When on, players can walk through sleeping players.
- `ForceResetWildDinos`: When on, wild creatures are reset (respawned).
- `NonPermanentDiseases`: In the disease/buff processing loop: when on, buffs flagged as permanent are skipped, so diseases don't stay permanent.
- `PlayerCharacterStaminaDrainMultiplier`: Player stamina drain x this.
- `PlayerDamageMultiplier`: Player-dealt damage x this.
- `PreventDiseases`: When on, there are no diseases.
- `PreventOverworldBosses`: World-boss spawner and King Kaiju: when on, overworld bosses are not spawned.
- `PreventSpawnAnimations`: When on, the spawn animation bits are cleared: players appear without the wake-up animation.
- `RiderDinoCollision`: Affects collision between riders and creatures.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `bAllowVariantDinoPacks` (also GUS) † | ✔ | bool = False | switch | Variant dino packs allowed | Variant dino packs not allowed |
| `BaseTemperatureMultiplier` | ✔ × | float = 1 | ↑ base temperature | More / stronger | Less / weaker |
| `ConfigAddNPCSpawnEntriesContainer` |  | list = (empty) |  |  |  |
| `ConfigOverrideNPCSpawnEntriesContainer` |  | list = (empty) |  |  |  |
| `ConfigSubtractNPCSpawnEntriesContainer` |  | list = (empty) |  |  |  |
| `DinoClassDamageMultipliers` |  | list = (empty) |  |  |  |
| `DinoClassResistanceMultipliers` |  | list = (empty) |  |  |  |
| `DinoClassSpeedMultipliers` |  | list = (empty) |  |  |  |
| `DinoClassStaminaMultipliers` |  | list = (empty) |  |  |  |
| `DinoHairGrowthSpeedMultiplier` † | ✔ | float = 1 | ↑ speed | Faster | Slower |
| `DinoSpawnWeightMultipliers` |  | list = (empty) |  |  |  |
| `bDisableDinoRiding` † | ✔ | bool = False | switch | Dino riding off | Dino riding on |
| `bDisableTekLegsBoost` (also GUS) † | ✔ | bool = False | switch | Tek legs boost off | Tek legs boost on |
| `bEnablePlayerMoveThroughAlly` (also GUS) | ✗ | bool = False | switch | Player move through ally on | Player move through ally off |
| `bEnablePlayerMoveThroughSleeping` (also GUS) † | ✔ | bool = False | switch | Player move through sleeping on | Player move through sleeping off |
| `GachasOverTribeLimitPenalty` † | ✔ | float = 3 | ↑ penalty over gacha limit | Harsher penalty for gachas over the tribe limit | Milder |
| `HairGrowthSpeedMultiplier` † | ✔ | float = 0 | ↑ hair growth speed | Hair grows faster | Hair grows slower; 0 or less stops it |
| `MaxDinoBaseLevel` † | ✔ | int = 0 | ↑ max dino base level | Higher limit | Lower limit |
| `MaxFallSpeedMultiplier` | ✔ × | float = 1 | ↑ speed | Faster | Slower |
| `NPCReplacements` |  | list = (empty) |  |  |  |
| `NumGachasPerTribeForPenalty` † | ✔ | int = 100 | ↑ gachas before penalty | More gachas allowed before the penalty | Fewer |
| `PreventMimicDinoClassNames` |  | list = (empty) |  |  |  |
| `bPreventOverworldBosses` (also GUS) † | ✔ | bool = False | switch | Overworld bosses blocked | Overworld bosses allowed |
| `TamedDinoCharacterFoodDrainMultiplier` | ✗ | float = 1 | ↑ drain rate | Drains faster | Drains slower |
| `TamedDinoClassDamageMultipliers` |  | list = (empty) |  |  |  |
| `TamedDinoClassResistanceMultipliers` |  | list = (empty) |  |  |  |
| `TamedDinoClassSpeedMultipliers` |  | list = (empty) |  |  |  |
| `TamedDinoClassStaminaMultipliers` |  | list = (empty) |  |  |  |
| `TamedDinoTorporDrainMultiplier` | ✗ | float = 1 | ↑ drain rate | Drains faster | Drains slower |
| `WildDinoCharacterFoodDrainMultiplier` | ✗ | float = 1 | ↑ drain rate | Drains faster | Drains slower |
| `WildDinoTorporDrainMultiplier` | ✗ | float = 1 | ↑ drain rate | Drains faster | Drains slower |
| `WildFollowerSpawnChanceMultiplier` † | ✔ | float = 1 | ↑ wild follower spawn chance | More / higher | Less / lower |
| `WildFollowerSpawnCountMultiplier` † | ✔ | float = 1 | ↑ wild follower spawn count | More / higher | Less / lower |

Notes:

- `bAllowVariantDinoPacks`: When on, variant creature pack entries are used when spawning.
- `DinoHairGrowthSpeedMultiplier`: Sheep wool / Archelon algae: time to full = base time / this; higher = faster regrowth.
- `bDisableDinoRiding`: When on, creatures can't be ridden.
- `bDisableTekLegsBoost`: When on, the Tek run boost is blocked.
- `bEnablePlayerMoveThroughSleeping`: When on, players can walk through sleeping players.
- `GachasOverTribeLimitPenalty`: Gacha digestion time × this when the tribe has more gachas than NumGachasPerTribeForPenalty.
- `HairGrowthSpeedMultiplier`: Growth += dt x this / species time; 0 or less disables hair growth.
- `MaxDinoBaseLevel`: If > 0, wild creature base level is capped to this.
- `NumGachasPerTribeForPenalty`: Gacha: if ≥ 0, the tribe's gacha count above this triggers GachasOverTribeLimitPenalty.
- `bPreventOverworldBosses`: World-boss spawner and King Kaiju: when on, overworld bosses are not spawned.
- `WildFollowerSpawnChanceMultiplier`: Affects wild pack followers.
- `WildFollowerSpawnCountMultiplier`: Affects wild pack followers.

### Transfers & cluster

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `CrossARKAllowForeignDinoDownloads` † | ✔ | bool = False |  | switch | On: cross ark allow foreign dino downloads | Off |
| `MaxTributeCharacters` † | ✔ | int = 10 |  | ↑ max tribute characters | Higher limit | Lower limit |
| `MaxTributeDinos` † | ✔ | int = 20 |  | ↑ max tribute dinos | Higher limit | Lower limit |
| `MinimumDinoReuploadInterval` | ✔ lim | float = 0 |  | ↑ minimum dino reupload interval | Higher limit | Lower limit |
| `NoTributeDownloads` † | ✔ | bool = False |  | switch | No tribute downloads | Tribute downloads allowed |
| `PreventDownloadDinos` † | ✔ | bool = False |  | switch | Download dinos blocked | Download dinos allowed |
| `PreventDownloadSurvivors` † | ✔ | bool = False |  | switch | Download survivors blocked | Download survivors allowed |
| `PreventUploadDinos` † | ✔ | bool = False |  | switch | Upload dinos blocked | Upload dinos allowed |
| `PreventUploadSurvivors` † | ✔ | bool = False |  | switch | Upload survivors blocked | Upload survivors allowed |
| `TributeCharacterExpirationSeconds` † | ✔ | int = 0 |  | ↑ tribute character expiration seconds | Longer | Shorter |
| `TributeDinoExpirationSeconds` † | ✔ | int = 86400 |  | ↑ tribute dino expiration seconds | Longer | Shorter |

Notes:

- `CrossARKAllowForeignDinoDownloads`: When on, creatures from other ARKs/maps may be downloaded.
- `MaxTributeCharacters`: Passed to the upload-limit check for survivors.
- `MaxTributeDinos`: Passed to the upload-limit check for creatures.
- `NoTributeDownloads`: Stored inverted (downloads allowed); downloads proceed only when this is off.
- `PreventDownloadDinos`: When on, creature downloads are refused.
- `PreventDownloadSurvivors`: When on, survivor downloads are refused.
- `PreventUploadDinos`: When on, creature uploads are refused.
- `PreventUploadSurvivors`: When on, survivor uploads are refused.
- `TributeCharacterExpirationSeconds`: If > 0, uploaded survivors expire after this many seconds.
- `TributeDinoExpirationSeconds`: If > 0, uploaded creatures expire after this many seconds.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `bDisableCustomFoldersInTributeInventories` | ✗ | bool = False | switch | Custom folders in tribute inventories off | Custom folders in tribute inventories on |
| `PreventTransferForClassNames` |  | list = (empty) |  |  |  |

### Tribes & players

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `AutoCreateTribes` † | ✔ | bool = False |  | switch | On: auto create tribes | Off |
| `EnableAFKKickPlayerCountPercent` | ✗ | float = 0 |  | ↑ player-count threshold for AFK kicks | AFK kicks start only when the server is fuller | AFK kicks start at lower population |
| `EnableDeathTeamSpectator` † | ✔ | bool = False |  | switch | Death team spectator on | Death team spectator off |
| `bFilterCharacterNames` † | ✔ | bool = False |  | switch | On: filter character names | Off |
| `bFilterTribeNames` † | ✔ | bool = False |  | switch | On: filter tribe names | Off |
| `ImplantSuicideCD` | ✔ lim | float = 28800 | yes | ↑ implant suicide cooldown | Longer | Shorter |
| `KickIdlePlayersPeriod` | ✔ lim | float = 3600 | yes | ↑ idle time before kick | Idle players kicked later | Kicked sooner |
| `PreventTribeAlliances` | ✗ | bool = False |  | switch | Tribe alliances blocked | Tribe alliances allowed |
| `TribeNameChangeCooldown` | ✔ lim | float = 15 | yes | ↑ tribe name change cooldown | Longer | Shorter |
| `UseExclusiveList` † | ✔ | bool = False |  | switch | Use exclusive list | Don't use exclusive list |

Notes:

- `AutoCreateTribes`: When on, players are put into an automatically created tribe.
- `EnableDeathTeamSpectator`: When on, dead players can spectate their team.
- `bFilterCharacterNames`: When on, survivor names are filtered for bad words.
- `bFilterTribeNames`: When on, tribe names are filtered for bad words.
- `UseExclusiveList`: When on, only players on the exclusive list may join.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `bAllowDisablingSpectator` † | ✔ | bool = True | switch | Disabling spectator allowed | Disabling spectator not allowed |
| `bDisableNonTribePinAccess` † | ✔ | bool = False | switch | Non tribe pin access off | Non tribe pin access on |
| `bFreezeTribes` | ✗ | bool = False | switch | On: freeze tribes | Off |
| `MaxAlliancesPerTribe` | ✔ lim | int = 0 | ↑ max alliances per tribe | Higher limit | Lower limit |
| `MaxNumberOfPlayersInTribe` † | ✔ | int = 0 | ↑ max number of players in tribe | Higher limit | Lower limit |
| `MaxTribeLogs` | ✔ lim | int = 400 | ↑ max tribe logs | Higher limit | Lower limit |
| `MaxTribesPerAlliance` | ✔ lim | int = 0 | ↑ max tribes per alliance | Higher limit | Lower limit |
| `bSaveFormerTribeOwnerLog` † | ✔ | bool = True | switch | On: save former tribe owner log | Off |
| `TimePeriodToHideDisconnectedPlayers` † | ✔ | float = 0 | ↑ time before hiding | Logged-out bodies stay visible longer | Hidden sooner; 0 = never hidden |
| `TribeSlotReuseCooldown` | ✔ lim | float = 0 | ↑ tribe slot reuse cooldown | Longer | Shorter |
| `TribeTowerBonusMultiplier` † | ✔ | float = 1 | ↑ tribe tower bonus | More / higher | Less / lower |

Notes:

- `bAllowDisablingSpectator`: When on, players may disable spectator mode.
- `bDisableNonTribePinAccess`: When on, PIN codes can't be used by players outside the tribe.
- `MaxNumberOfPlayersInTribe`: Compared against tribe member count.
- `bSaveFormerTribeOwnerLog`: When on, the former tribe's log is kept for the new owner.
- `TimePeriodToHideDisconnectedPlayers`: If > 0, a disconnected player's body is hidden after this many seconds.
- `TribeTowerBonusMultiplier`: Tribe tower bonus × this.

### Chat, UI & admin

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `AdminLogging` † | ✔ | bool = False |  | switch | On: admin logging | Off |
| `AllowHideDamageSourceFromLogs` † | ✔ | bool = True | yes | switch | Hide damage source from logs allowed | Hide damage source from logs not allowed |
| `AllowHitMarkers` † | ✔ | bool = True | yes | switch | Hit markers allowed | Hit markers not allowed |
| `AllowSharedConnections` † | ✗ | bool = True |  | switch | Shared connections allowed | Shared connections not allowed |
| `AllowThirdPersonPlayer` † | ✔ | bool = True | yes | switch | Third person player allowed | Third person player not allowed |
| `AlwaysNotifyPlayerLeft` | ✗ | bool = False |  | switch | On: always notify player left | Off |
| `AutoSavePeriodMinutes` † | ✔ | float = 15 | yes | ↑ time between saves | Saves less often (more progress lost on crash) | Saves more often |
| `ChatLogFileSplitIntervalSeconds` | ✗ | int = 45 |  | ↑ chat log file split interval seconds | Longer | Shorter |
| `ChatLogFlushIntervalSeconds` | ✗ | int = 15 |  | ↑ chat log flush interval seconds | Longer | Shorter |
| `ChatLogMaxAgeInDays` | ✗ | int = 5 |  | ↑ chat log max age in days | Longer | Shorter |
| `DontAlwaysNotifyPlayerJoined` † | ✗ | bool = False |  | switch | Don't always notify player joined | Always notify player joined |
| `DontRestoreBackup` | ✗ | bool = False |  | switch | Don't restore backup | Restore backup |
| `bFilterChat` † | ✔ | bool = False |  | switch | On: filter chat | Off |
| `GlobalVoiceChat` † | ✔ | bool = False |  | switch | Voice heard server-wide | Proximity voice chat |
| `LogChatMessages` | ✗ | bool = False |  | switch | On: log chat messages | Off |
| `ProximityChat` † | ✔ | bool = False |  | switch | On: proximity chat | Off |
| `ProximityChatInnerRadius` † | ✔ | float = 500 |  | ↑ proximity chat inner radius | Larger area | Smaller area |
| `ProximityChatOuterRadius` † | ✔ | float = 5000 |  | ↑ proximity chat outer radius | Larger area | Smaller area |
| `RCONEnabled` | ✗ | bool = False |  | switch | On: RCON enabled | Off |
| `RCONPort` | ✗ | int = 27020 | yes | port | RCON listens on this TCP port (set from RCON_PORT) | — |
| `RCONServerGameLogBuffer` | ✔ lim | float = 600 | yes | ↑ log lines kept for RCON | More game-log lines available to RCON | Fewer |
| `ServerCrosshair` † | ✔ | bool = True | yes | switch | On: server crosshair | Off |
| `ServerForceNoHUD` † | ✔ | bool = False |  | switch | On: server force no HUD | Off |
| `ShowFloatingDamageText` † | ✔ | bool = False |  | switch | Floating damage text shown | Floating damage text hidden |
| `ShowMapPlayerLocation` † | ✔ | bool = True | yes | switch | Map player location shown | Map player location hidden |
| `UpdateAllowedCheatersInterval` | ✗ | float = 600 |  | ↑ update allowed cheaters interval | Longer | Shorter |
| `UseCharacterTracker` † | ✔ | bool = True |  | switch | Use character tracker | Don't use character tracker |

Notes:

- `AdminLogging`: When on, admin commands are logged.
- `AllowHideDamageSourceFromLogs`: When on, the damage source can be hidden.
- `AllowHitMarkers`: When on, hit markers are sent to the attacker.
- `AllowSharedConnections`: Load not confirmed.
- `AllowThirdPersonPlayer`: Off restricts players to first-person view.
- `AutoSavePeriodMinutes`: World save when elapsed minutes reach this.
- `DontAlwaysNotifyPlayerJoined`: Load not confirmed.
- `bFilterChat`: When on, chat is filtered for bad words.
- `GlobalVoiceChat`: True = voice is heard server-wide, False = proximity voice.
- `ProximityChat`: When on, text chat is proximity-based.
- `ProximityChatInnerRadius`: Voice range: full volume within this distance.
- `ProximityChatOuterRadius`: Voice range: inaudible beyond this distance.
- `ServerCrosshair`: When on, the crosshair is drawn.
- `ServerForceNoHUD`: When on, players get no HUD.
- `ShowFloatingDamageText`: When on, damage numbers are sent to the attacker.
- `ShowMapPlayerLocation`: When on, players see their own position on the map.
- `UseCharacterTracker`: Enables the character tracker (tracked creatures and waypoints). Can also be toggled at runtime by an admin command.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `bAllowChatFromDeadNonAdmins` † | ✔ | bool = True | switch | Chat from dead non admins allowed | Chat from dead non admins not allowed |
| `CheatTeleportLocations` |  | list = (TeleportName="testpoi1",TeleportLocation=(X=42069.0,Y=42069.0,Z=42069.0)) |  |  |  |
| `bDisableBuffSavePlayerDataOnSaveWorld` † | ✔ | bool = False | switch | Buff save player data on save world off | Buff save player data on save world on |
| `bDisablePhotoMode` † | ✔ | bool = False | switch | Photo mode off | Photo mode on |
| `bDoAutomatedModValidationModeration` † | ✗ | bool = False | switch | On: do automated mod validation moderation | Off |
| `bFailedWaterDinoSpawnLogEnabled` † | ✔ | bool = False | switch | On: failed water dino spawn log enabled | Off |
| `bGameplayLogEnabled` | ✗ | bool = False | switch | On: gameplay log enabled | Off |
| `ModValidationFailureWarnings` | ✗ | int = 1 | ↑ warnings before action | More warnings before mod-validation action | Fewer |
| `PhotoModeRangeLimit` † | ✔ | float = 3000 | ↑ photo mode range limit | Larger area | Smaller area |
| `RTSMaxRangeFromPlayerCharacterScale` † | ✔ | float = 1 | ↑ RTS max range from player character scale | Larger area | Smaller area |
| `RTSModeNumSelectableDinosScale` | ✗ | float = 1 | ↑ RTS mode num selectable dinos scale | More / stronger | Less / weaker |
| `bServerGameLogEnabled` † | ✔ | bool = False | switch | On: server game log enabled | Off |
| `bShowCreativeMode` | ✗ | bool = False | switch | Creative mode shown | Creative mode hidden |

Notes:

- `bAllowChatFromDeadNonAdmins`: When on, dead players (non-admins) may still chat.
- `bDisableBuffSavePlayerDataOnSaveWorld`: Companion buffs: when on, buff data isn't saved with the player on world save.
- `bDisablePhotoMode`: When on, photo mode is off.
- `bDoAutomatedModValidationModeration`: Not applied when set in Game.ini.
- `bFailedWaterDinoSpawnLogEnabled`: When on, failed water-creature spawns are logged.
- `PhotoModeRangeLimit`: Photo mode drone / targeting: max range; 0 = unlimited.
- `RTSMaxRangeFromPlayerCharacterScale`: Divides the RTS command range falloff; 0 or less disables the range check.
- `bServerGameLogEnabled`: When on, events are written to the server game log (also on with -servergamelog).

### World, time & events

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `DayCycleSpeedScale` † | ✔ | float = 1 |  | ↑ day/night cycle speed | Days pass faster | Days pass slower |
| `DayTimeSpeedScale` † | ✔ | float = 1 |  | ↑ daytime speed | Daytime passes faster (shorter days) | Longer days |
| `DisableWeatherFog` | ✗ | bool = False |  | switch | Weather fog off | Weather fog on |
| `ElementalIslandShutdownFrequencyMultiplier` | ✗ | float = 1 | yes | ↑ elemental island shutdown frequency | More / stronger | Less / weaker |
| `ExtinctionEventTimeInterval` † | ✔ ÷ | float = 0 |  | ↑ extinction event time interval | Longer | Shorter |
| `NightTimeSpeedScale` † | ✔ | float = 1 |  | ↑ night speed | Night passes faster (shorter nights) | Longer nights |
| `OverrideStartTime` † | ✔ | bool = False |  | switch | Server starts at StartTimeHour | Map default start time |
| `StartTimeHour` † | ✔ | int = -1 | yes | ↑ starting hour | Server starts later in the day (with OverrideStartTime) | Earlier |

Notes:

- `DayCycleSpeedScale`: Time speed = the map's own day-cycle factor × this × the day or night factor.
- `DayTimeSpeedScale`: Daytime speed = map's DayTimeSpeedScale × this (× DayCycleSpeedScale). Maps such as The Center set their own factor.
- `ExtinctionEventTimeInterval`: Managed by the game together with NextExtinctionEventUTC; don't set by hand.
- `NightTimeSpeedScale`: Night speed = map's NightTimeSpeedScale × this (× DayCycleSpeedScale). Maps such as Scorched Earth set their own factor.
- `OverrideStartTime`: When on, StartTimeHour is used as the starting hour; when off it is ignored.
- `StartTimeHour`: Only applies when OverrideStartTime is on; otherwise the map's start time is used.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `bDisableWorldBuffs` | ✗ | bool = False | switch | World buffs off | World buffs on |
| `bEnableWorldBuffScaling` † | ✔ | bool = False | switch | World buff scaling on | World buff scaling off |
| `ExtinctionEventTimeInterval` (also GUS) † | ✔ ÷ | float = 0 | ↑ extinction event time interval | Longer | Shorter |
| `NextExtinctionEventUTC` † | ✗ | int = 0 | timestamp | Next Extinction event time (UTC, set by the game) | — |
| `ValgueroMemorialEntries` |  | string = — |  |  |  |
| `WorldBuffScalingEfficacy` | ✔ × | float = 0 | ↑ world buff strength | World buffs scale more strongly | Weaker |

Notes:

- `bEnableWorldBuffScaling`: When on, world buff scaling (WorldBuffScalingEfficacy) is applied; off = 0.
- `ExtinctionEventTimeInterval`: Managed by the game together with NextExtinctionEventUTC; don't set by hand.
- `NextExtinctionEventUTC`: Don't set by hand: a time in the past starts an extinction event at once and shuts the server down.

### Missions, outposts & hexagons

**GameUserSettings.ini `[ServerSettings]`**

| Key | Conf. | Type = default | Written | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|---|
| `EnforceMilestoneTaskOrder` | ✗ | bool = False |  | switch | On: enforce milestone task order | Off |
| `MaxHexagonsPerCharacter` † | ✔ | int = 2000000000 |  | ↑ max hexagons per character | Higher limit | Lower limit |
| `MaxHexagonsToAward` † | ✔ | int = -1 |  | ↑ max hexagons to award | Higher limit | Lower limit |
| `OfficialDisableGenesisMissions` | ✗ | bool = False |  | switch | On: official disable genesis missions | Off |
| `OutpostSigilRewardMultiplier` † | ✔ | float = 1 | yes | ↑ outpost sigil reward | More / higher | Less / lower |
| `OverrideSecondsUntilBuriedTreasureAutoReveals` † | ✔ | int = 1209600 | yes | ↑ override seconds until buried treasure auto reveals | Longer | Shorter |

Notes:

- `MaxHexagonsPerCharacter`: Negative = no cap; otherwise hexagon balance is clamped to this.
- `MaxHexagonsToAward`: Negative = no cap; otherwise caps hexagons awarded.
- `OutpostSigilRewardMultiplier`: Lost Colony outposts: sigils distributed × this.
- `OverrideSecondsUntilBuriedTreasureAutoReveals`: If > 0, seconds until a buried treasure reveals itself.

**Game.ini `[/Script/ShooterGame.ShooterGameMode]`**

| Key | Conf. | Type = default | Raising changes | Raised / True | Lowered / False |
|---|---|---|---|---|---|
| `BaseHexagonRewardMultiplier` | ✔ × | float = 1 | ↑ base hexagon reward | More / higher | Less / lower |
| `bDisableGenesisMissions` † | ✔ | bool = False | switch | Genesis missions off | Genesis missions on |
| `bDisableHexagonStore` † | ✔ | bool = False | switch | Hexagon store off | Hexagon store on |
| `bDontEnforceMilestoneTaskOrder` | ✗ | bool = False | switch | Don't enforce milestone task order | Enforce milestone task order |
| `HexagonCostMultiplier` † | ✔ | float = 1 | ↑ cost | More expensive | Cheaper |
| `HexagonRewardMultiplier` † | ✔ | float = 1 | ↑ hexagon reward | More / higher | Less / lower |
| `bHexStoreAllowOnlyEngramTradeOption` † | ✔ | bool = False | switch | On: hex store allow only engram trade option | Off |
| `MaxActiveCityOutposts` † | ✔ | int = 8 | ↑ max active city outposts | Higher limit | Lower limit |
| `MaxActiveCityOutpostsSP` † | ✔ | int = 8 | ↑ max active city outposts SP | Higher limit | Lower limit |
| `MaxActiveGenesisLandOutposts` † | ✔ | int = 25 | ↑ max active genesis land outposts | Higher limit | Lower limit |
| `MaxActiveOutposts` † | ✔ | int = 14 | ↑ max active outposts | Higher limit | Lower limit |
| `MaxActiveOutpostsSP` † | ✔ | int = 14 | ↑ max active outposts SP | Higher limit | Lower limit |
| `MaxHexagonsPerCharacter` (also GUS) † | ✔ | int = 2000000000 | ↑ max hexagons per character | Higher limit | Lower limit |
| `MaxHexagonsToAward` (also GUS) † | ✔ | int = -1 | ↑ max hexagons to award | Higher limit | Lower limit |
| `bOutpostsNoInitialDinosSP` † | ✔ | bool = False | switch | On: outposts no initial dinos SP | Off |

Notes:

- `bDisableGenesisMissions`: When on, Genesis missions are skipped.
- `bDisableHexagonStore`: HLNA companion: when on, the Hexagon store entries are disabled.
- `HexagonCostMultiplier`: Hexagon store cost x this.
- `HexagonRewardMultiplier`: Hexagon reward x this.
- `bHexStoreAllowOnlyEngramTradeOption`: When on, the Hexagon store only offers items flagged as engram trades.
- `MaxActiveCityOutposts`: Lost Colony city outposts: maximum active at once = this.
- `MaxActiveCityOutpostsSP`: Lost Colony city outposts (single player): maximum active at once = this.
- `MaxActiveGenesisLandOutposts`: Genesis land outposts: maximum active at once = this.
- `MaxActiveOutposts`: Genesis water and Lost Colony outposts: maximum active at once = this.
- `MaxActiveOutpostsSP`: Outposts (single player): maximum active at once = this.
- `MaxHexagonsPerCharacter`: Negative = no cap; otherwise hexagon balance is clamped to this.
- `MaxHexagonsToAward`: Negative = no cap; otherwise caps hexagons awarded.
- `bOutpostsNoInitialDinosSP`: Lost Colony outposts (single player): when on, no initial creatures spawn.


## Other sections

| File | Section | Keys |
|---|---|---|
| `GameUserSettings.ini` | `[SessionSettings]` | `SessionName` (set from `SESSION_NAME`) |
| `GameUserSettings.ini` | `[/Script/Engine.GameSession]` | `MaxPlayers` (default 70) |
| `GameUserSettings.ini` | `[MessageOfTheDay]` | `Message`, `Duration` |
