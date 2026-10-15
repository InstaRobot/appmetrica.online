"use client";

import { cn } from "cn";
import type * as React from "react";
import { createContext, useContext } from "react";

// Режим карточек на телефоне (< md): строка — карточка с рамкой, шапка таблицы — только для скринридеров,
// подпись ячейки — из data-label. Смена display лишает браузерную таблицу её семантики, поэтому в этом режиме
// роли table/rowgroup/row/columnheader/cell проставлены явно. На md и шире — обычная таблица.
const StackContext = createContext(false);

function useStacked(): boolean {
  return useContext(StackContext);
}

type TableProps = React.ComponentProps<"table"> & {
  /** На телефоне строки — карточки вместо горизонтальной прокрутки; подписи ячеек — TableCell label. */
  stackOnMobile?: boolean;
};

function Table({ className, stackOnMobile = false, ...props }: TableProps) {
  return (
    <StackContext value={stackOnMobile}>
      <div
        data-slot="table-container"
        className={cn("relative w-full", stackOnMobile ? "md:overflow-x-auto" : "overflow-x-auto")}
      >
        <table
          data-slot="table"
          {...(stackOnMobile ? { role: "table" } : {})}
          className={cn(
            "w-full caption-bottom text-sm",
            stackOnMobile && "max-md:block",
            className,
          )}
          {...props}
        />
      </div>
    </StackContext>
  );
}

function TableHeader({ className, ...props }: React.ComponentProps<"thead">) {
  const stacked = useStacked();
  return (
    <thead
      data-slot="table-header"
      {...(stacked ? { role: "rowgroup" } : {})}
      className={cn("[&_tr]:border-b", stacked && "max-md:sr-only", className)}
      {...props}
    />
  );
}

function TableBody({ className, ...props }: React.ComponentProps<"tbody">) {
  const stacked = useStacked();
  return (
    <tbody
      data-slot="table-body"
      {...(stacked ? { role: "rowgroup" } : {})}
      className={cn(
        // У карточек рамка со всех сторон, в том числе у последней.
        stacked
          ? "max-md:flex max-md:flex-col max-md:gap-3 md:[&_tr:last-child]:border-0"
          : "[&_tr:last-child]:border-0",
        className,
      )}
      {...props}
    />
  );
}

function TableFooter({ className, ...props }: React.ComponentProps<"tfoot">) {
  const stacked = useStacked();
  return (
    <tfoot
      data-slot="table-footer"
      {...(stacked ? { role: "rowgroup" } : {})}
      className={cn(
        "border-t bg-muted/50 font-medium [&>tr]:last:border-b-0",
        stacked && "max-md:block",
        className,
      )}
      {...props}
    />
  );
}

function TableRow({ className, ...props }: React.ComponentProps<"tr">) {
  const stacked = useStacked();
  return (
    <tr
      data-slot="table-row"
      {...(stacked ? { role: "row" } : {})}
      className={cn(
        "border-b transition-colors hover:bg-muted/50 has-aria-expanded:bg-muted/50 data-[state=selected]:bg-muted",
        stacked &&
          "max-md:grid max-md:grid-cols-2 max-md:gap-x-4 max-md:gap-y-3 max-md:rounded-lg max-md:border max-md:p-3",
        className,
      )}
      {...props}
    />
  );
}

function TableHead({ className, ...props }: React.ComponentProps<"th">) {
  const stacked = useStacked();
  return (
    <th
      data-slot="table-head"
      {...(stacked ? { role: "columnheader" } : {})}
      className={cn(
        "h-10 px-2 text-left align-middle font-medium whitespace-nowrap text-foreground [&:has([role=checkbox])]:pr-0",
        className,
      )}
      {...props}
    />
  );
}

type TableCellProps = React.ComponentProps<"td"> & {
  /** Подпись значения в карточке (обычно — заголовок колонки). Ячейка без подписи — на всю ширину карточки. */
  label?: string;
};

function TableCell({ className, label, ...props }: TableCellProps) {
  const stacked = useStacked();
  return (
    <td
      data-slot="table-cell"
      {...(label !== undefined ? { "data-label": label } : {})}
      {...(stacked ? { role: "cell" } : {})}
      className={cn(
        "p-2 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0",
        stacked &&
          "max-md:block max-md:min-w-0 max-md:p-0 max-md:text-left max-md:wrap-break-word max-md:whitespace-normal max-md:empty:hidden",
        stacked &&
          (label
            ? "max-md:before:block max-md:before:font-sans max-md:before:text-xs max-md:before:font-normal max-md:before:text-fg-muted max-md:before:content-[attr(data-label)]"
            : "max-md:col-span-full"),
        className,
      )}
      {...props}
    />
  );
}

function TableCaption({ className, ...props }: React.ComponentProps<"caption">) {
  return (
    <caption
      data-slot="table-caption"
      className={cn("mt-4 text-sm text-muted-foreground", className)}
      {...props}
    />
  );
}

export { Table, TableBody, TableCaption, TableCell, TableFooter, TableHead, TableHeader, TableRow };
