// oxlint-disable react/only-export-components -- The provider and its timezone conversion API must share one module.
import { getTimeZones } from "@vvo/tzdb";
import { createContext, useContext, useState, type ReactNode } from "react";

const storageKey = "temporal-lens:time-zone";
const timeZoneData = getTimeZones({ includeUtc: true });
const timeZoneAliases = new Map<string, string>();

for (const timeZone of timeZoneData) {
  const name = timeZone.name === "Etc/UTC" ? "UTC" : timeZone.name;
  for (const alias of timeZone.group) timeZoneAliases.set(alias, name);
}

function canonicalTimeZone(timeZone: string) {
  return timeZoneAliases.get(timeZone) ?? timeZone;
}

const browserTimeZone = canonicalTimeZone(Intl.DateTimeFormat().resolvedOptions().timeZone);

export interface TimeZoneOption {
  name: string;
}

const regionOrder = ["Universal", "Africa", "America", "Antarctica", "Asia", "Europe", "Oceania"];

/**
 * Maintained IANA timezone metadata, grouped by geographical region for the picker.
 * @vvo/tzdb also omits zones unavailable in the current browser at runtime.
 */
export const timeZoneRegions = Object.entries(
  timeZoneData.reduce<Record<string, TimeZoneOption[]>>((regions, timeZone) => {
    const name = timeZone.name === "Etc/UTC" ? "UTC" : timeZone.name;
    const region = timeZone.name === "Etc/UTC" ? "Universal" : timeZone.continentName || "Other";
    const option = { name };
    (regions[region] ??= []).push(option);
    return regions;
  }, {}),
)
  .sort(([left], [right]) => {
    const leftOrder = regionOrder.indexOf(left);
    const rightOrder = regionOrder.indexOf(right);
    return (
      (leftOrder === -1 ? regionOrder.length : leftOrder) -
        (rightOrder === -1 ? regionOrder.length : rightOrder) || left.localeCompare(right)
    );
  })
  .map(([region, zones]) => ({
    region,
    zones: zones.sort((left, right) => left.name.localeCompare(right.name)),
  }));

export const timeZones = timeZoneRegions.flatMap(({ zones }) => zones.map(({ name }) => name));

interface TimeZoneContextValue {
  timeZone: string;
  setTimeZone: (timeZone: string) => void;
}

const TimeZoneContext = createContext<TimeZoneContextValue | undefined>(undefined);

function isTimeZone(timeZone: string) {
  return timeZones.includes(timeZone);
}

function savedTimeZone() {
  const saved = window.localStorage.getItem(storageKey);
  const timeZone = saved === null ? browserTimeZone : canonicalTimeZone(saved);
  return isTimeZone(timeZone) ? timeZone : browserTimeZone;
}

/** Keeps timestamp presentation and timestamp-filter input in the same persisted IANA time zone. */
export function TimeZoneProvider({ children }: { children: ReactNode }) {
  const [timeZone, setCurrentTimeZone] = useState(savedTimeZone);
  const setTimeZone = (nextTimeZone: string) => {
    if (!isTimeZone(nextTimeZone)) return;
    window.localStorage.setItem(storageKey, nextTimeZone);
    setCurrentTimeZone(nextTimeZone);
  };

  return <TimeZoneContext value={{ timeZone, setTimeZone }}>{children}</TimeZoneContext>;
}

export function useTimeZone() {
  const value = useContext(TimeZoneContext);
  if (value === undefined) {
    throw new Error("useTimeZone must be used within a TimeZoneProvider.");
  }
  return value;
}

function zonedParts(date: Date, timeZone: string) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  }).formatToParts(date);
  return Object.fromEntries(
    parts.filter((part) => part.type !== "literal").map((part) => [part.type, Number(part.value)]),
  ) as Record<"year" | "month" | "day" | "hour" | "minute" | "second", number>;
}

function pad(value: number) {
  return String(value).padStart(2, "0");
}

/** Formats an instant as the wall-clock value used by the timestamp filter editor. */
export function zonedDateTimeValue(date: Date, timeZone: string) {
  const parts = zonedParts(date, timeZone);
  return `${parts.year}-${pad(parts.month)}-${pad(parts.day)}T${pad(parts.hour)}:${pad(parts.minute)}`;
}

/** Converts an editor wall-clock value in `timeZone` into the instant sent to the API. */
export function dateFromZonedDateTime(value: string, timeZone: string): Date | undefined {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value);
  if (match === null) return undefined;
  const [year, month, day, hour, minute] = match.slice(1).map(Number);
  const wallClock = Date.UTC(year, month - 1, day, hour, minute);
  const offsetAt = (instant: Date) => {
    const parts = zonedParts(instant, timeZone);
    return (
      Date.UTC(parts.year, parts.month - 1, parts.day, parts.hour, parts.minute) - instant.getTime()
    );
  };
  let result = new Date(wallClock - offsetAt(new Date(wallClock)));
  // Re-evaluate once in case the selected wall-clock time crosses a DST boundary.
  result = new Date(wallClock - offsetAt(result));
  return Number.isNaN(result.valueOf()) ? undefined : result;
}

/** Creates a browser-local calendar date with the same visible date/time as a zoned editor value. */
export function calendarDateFromZonedDateTime(value: string, timeZone: string): Date | undefined {
  const instant = dateFromZonedDateTime(value, timeZone);
  if (instant === undefined) return undefined;
  const parts = zonedParts(instant, timeZone);
  return new Date(parts.year, parts.month - 1, parts.day, parts.hour, parts.minute);
}

/** Combines a Calendar day and time input into the timezone-independent editor representation. */
export function zonedDateTimeFromCalendarDate(date: Date, time: string) {
  const [hour, minute] = time.split(":").map(Number);
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(hour || 0)}:${pad(minute || 0)}`;
}

export function formatZonedDateTime(date: Date, timeZone: string) {
  return new Intl.DateTimeFormat(undefined, {
    timeZone,
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

/** Returns the UTC offset that applies to this instant in the selected zone. */
export function timeZoneOffsetLabel(date: Date, timeZone: string) {
  const parts = zonedParts(date, timeZone);
  const offsetMinutes = Math.round(
    (Date.UTC(parts.year, parts.month - 1, parts.day, parts.hour, parts.minute, parts.second) -
      date.getTime()) /
      60_000,
  );
  if (offsetMinutes === 0) return "UTC";
  const sign = offsetMinutes > 0 ? "+" : "-";
  const absoluteMinutes = Math.abs(offsetMinutes);
  return `UTC${sign}${pad(Math.floor(absoluteMinutes / 60))}:${pad(absoluteMinutes % 60)}`;
}
