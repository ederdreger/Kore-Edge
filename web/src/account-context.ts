import { createContext, useContext } from 'react';
export type PanelUser={id:string;username:string;role:'admin'|'viewer';disabled:boolean;revision:number};
type Account={user:PanelUser|null;request:(method:string,path:string,body?:unknown)=>Promise<Response>;reload:()=>Promise<void>;logout:()=>Promise<void>};
export const AccountContext=createContext<Account|null>(null);
export const useAccount=()=>useContext(AccountContext);
