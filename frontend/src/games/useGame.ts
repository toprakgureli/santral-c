// useGame: one match, kept in step with the room's live stream. A "game"
// event refetches the viewer-specific state; strokes and frames arrive as
// raw payloads for the drawing board and the hockey table. The clock is
// counted down locally between updates. When the setting allows, a call
// on this side pauses the match automatically.

import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "@/api/client";
import type { TeamsEvent } from "@/api/types";
import { gamesApi } from "@/games/api";
import type { GameView } from "@/games/types";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";
import { useTeams } from "@/teams/TeamsContext";

export interface GameHandle {
  game: GameView | null;
  error: string | null;
  seconds: number;
  refresh: () => Promise<void>;
  act: (action: string, payload?: unknown) => Promise<void>;
  join: () => Promise<void>;
  leave: () => Promise<void>;
  start: () => Promise<void>;
  cancel: () => Promise<void>;
  onStroke: (fn: (payload: unknown) => void) => () => void;
  onFrame: (fn: (payload: unknown) => void) => () => void;
  busy: boolean;
}

export function useGame(id: number | null, pauseOnCall: boolean): GameHandle {
  const teams = useTeams();
  const phone = useSoftphoneContext();
  const [game, setGame] = useState<GameView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [seconds, setSeconds] = useState(0);
  const [busy, setBusy] = useState(false);
  const strokeFns = useRef(new Set<(p: unknown) => void>());
  const frameFns = useRef(new Set<(p: unknown) => void>());
  const versionRef = useRef(0);

  const refresh = useCallback(async () => {
    if (!id) return;
    try {
      const g = await gamesApi.get(id);
      if (g.version >= versionRef.current) {
        versionRef.current = g.version;
        setGame(g);
        setSeconds(g.secondsLeft);
      }
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Oyun alınamadı.");
    }
  }, [id]);

  useEffect(() => {
    versionRef.current = 0;
    setGame(null);
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!id) return;
    return teams.subscribe((e: TeamsEvent) => {
      if (e.gameId !== id) return;
      if (e.type === "game") void refresh();
      else if (e.type === "game.stroke") strokeFns.current.forEach((fn) => fn(e.payload));
      else if (e.type === "game.frame") frameFns.current.forEach((fn) => fn(e.payload));
    });
  }, [id, teams, refresh]);

  // Local countdown between server updates.
  useEffect(() => {
    if (!game || game.status !== "playing" || game.paused || game.secondsLeft <= 0) return;
    const t = window.setInterval(() => setSeconds((s) => Math.max(0, s - 1)), 1000);
    return () => window.clearInterval(t);
  }, [game]);

  // A call on this side pauses the match for everyone.
  const inCall = phone.status === "in-call" || phone.status === "incoming" || phone.status === "calling" || phone.status === "ringing" || phone.status === "held";
  const pausedByMe = useRef(false);
  useEffect(() => {
    if (!id || !game || game.status !== "playing" || !game.joined || !pauseOnCall) return;
    if (inCall && !pausedByMe.current) {
      pausedByMe.current = true;
      void gamesApi.pause(id, true).catch(() => undefined);
    } else if (!inCall && pausedByMe.current) {
      pausedByMe.current = false;
      void gamesApi.pause(id, false).catch(() => undefined);
    }
  }, [inCall, id, game, pauseOnCall]);

  const wrap = useCallback(
    (fn: () => Promise<GameView | void>) => async () => {
      if (!id) return;
      setBusy(true);
      try {
        const g = await fn();
        if (g && g.version >= versionRef.current) {
          versionRef.current = g.version;
          setGame(g);
          setSeconds(g.secondsLeft);
        }
        setError(null);
      } catch (e) {
        setError(e instanceof ApiError ? e.message : "İşlem yapılamadı.");
      } finally {
        setBusy(false);
      }
    },
    [id],
  );

  return {
    game,
    error,
    seconds,
    refresh,
    busy,
    act: async (action, payload) => {
      if (!id) return;
      try {
        const g = await gamesApi.act(id, action, payload ?? {});
        if (g.version >= versionRef.current) {
          versionRef.current = g.version;
          setGame(g);
          setSeconds(g.secondsLeft);
        }
        setError(null);
      } catch (e) {
        setError(e instanceof ApiError ? e.message : "Hamle yapılamadı.");
        throw e;
      }
    },
    join: wrap(() => gamesApi.join(id!)),
    leave: wrap(() => gamesApi.leave(id!)),
    start: wrap(() => gamesApi.start(id!)),
    cancel: wrap(() => gamesApi.cancel(id!).then(() => refresh())),
    onStroke: (fn) => {
      strokeFns.current.add(fn);
      return () => {
        strokeFns.current.delete(fn);
      };
    },
    onFrame: (fn) => {
      frameFns.current.add(fn);
      return () => {
        frameFns.current.delete(fn);
      };
    },
  };
}
