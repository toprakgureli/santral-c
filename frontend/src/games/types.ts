// Mini games: shared types between the room card, the game window, the
// leaderboard and the administration screen.

export interface GameMeta {
  key: string;
  name: string;
  tagline: string;
  how: string;
  minPlayers: number;
  maxPlayers: number;
  itemKind: string;
  itemLabel: string;
  itemHint: string;
  minItems: number;
  defaultRounds: number;
  roundsLabel: string;
  defaultSeconds: number;
  secondsLabel: string;
  realtime: boolean;
  joinLate: boolean;
  icon: string;
}

export interface GameSettings {
  enabled: boolean;
  pauseOnCall: boolean;
  breakOnly: boolean;
}

export interface GamesConfig extends GameSettings {
  canManage: boolean;
  kinds: GameMeta[];
  itemCounts: Record<string, number>;
}

export interface GameItem {
  id: number;
  kind: string;
  text: string;
  answer: string;
  options: string[];
  seconds: number;
  active: boolean;
}

export interface GamePlayer {
  id: number;
  name: string;
  team: number;
  score: number;
  left?: boolean;
}

export interface GameView {
  id: number;
  groupId: number;
  kind: string;
  kindName: string;
  status: "lobby" | "playing" | "finished" | "cancelled";
  hostId: number;
  players: GamePlayer[];
  config: { rounds: number; seconds: number };
  version: number;
  paused: boolean;
  pausedBy: string[];
  secondsLeft: number;
  winners: number[];
  joined: boolean;
  isHost: boolean;
  canManage: boolean;
  minPlayers: number;
  maxPlayers: number;
  joinLate: boolean;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  data: any;
  createdAt: string;
}

export interface LeaderRow {
  userId: number;
  name: string;
  played: number;
  wins: number;
  points: number;
}

export interface GameRecord {
  played: number;
  wins: number;
  points: number;
  kinds: { kind: string; played: number; wins: number; points: number }[];
}
