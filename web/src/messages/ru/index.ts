import type { Messages } from "../index";
import { apps } from "./apps";
import { auth } from "./auth";
import { common } from "./common";
import { demo } from "./demo";
import { demoData } from "./demoData";
import { docs } from "./docs";
import { errors } from "./errors";
import { integration } from "./integration";
import { landing } from "./landing";
import { layout } from "./layout";
import { metrics } from "./metrics";
import { portfolio } from "./portfolio";
import { profiles } from "./profiles";
import { realtime } from "./realtime";
import { reports } from "./reports";
import { retention } from "./retention";
import { settings } from "./settings";
import { system } from "./system";

export const ru: Messages = {
  common,
  layout,
  landing,
  auth,
  system,
  demo,
  portfolio,
  reports,
  realtime,
  retention,
  metrics,
  apps,
  integration,
  errors,
  settings,
  profiles,
  demoData,
  docs,
};
