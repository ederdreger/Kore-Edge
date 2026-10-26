// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App } from './App';

beforeEach(()=>{
 window.history.replaceState(null,'','/#dashboard');
 Object.defineProperty(HTMLDialogElement.prototype,'showModal',{configurable:true,value:function(){this.setAttribute('open','');}});
 Object.defineProperty(HTMLDialogElement.prototype,'close',{configurable:true,value:function(){this.removeAttribute('open');}});
});
afterEach(()=>{cleanup();vi.unstubAllGlobals();});
describe('painel acessível e sem efeitos operacionais',()=>{
 it('marca demonstração, filtra canais, abre detalhes e não consulta APIs',async()=>{
  const fetch=vi.fn();vi.stubGlobal('fetch',fetch);const user=userEvent.setup();render(<App initialDemo/>);
  expect(screen.getByText('DEMONSTRAÇÃO · DADOS SIMULADOS')).toBeTruthy();expect(screen.getAllByRole('row')).toHaveLength(9);
  await user.type(screen.getByRole('textbox',{name:'Buscar canal'}),'ESPN');expect(screen.getAllByRole('row')).toHaveLength(2);
  await user.clear(screen.getByRole('textbox',{name:'Buscar canal'}));await user.selectOptions(screen.getByRole('combobox',{name:'Filtrar por status'}),'attention');
  expect(screen.getAllByRole('row')).toHaveLength(2);await user.click(screen.getByRole('button',{name:'Detalhes de Discovery'}));
  expect(screen.getByRole('dialog',{name:'Discovery'})).toBeTruthy();await user.click(screen.getByRole('button',{name:'Fechar diálogo'}));
  await user.click(screen.getByRole('button',{name:'Adicionar Canal'}));expect(screen.getByRole('dialog',{name:'Adicionar canal'}).textContent).toContain('somente leitura');expect(fetch).not.toHaveBeenCalled();
 });
 it('mantém busca por teclado, skip-link e navegação de páginas',async()=>{
  vi.stubGlobal('fetch',vi.fn());const user=userEvent.setup();render(<App initialDemo/>);
  fireEvent.keyDown(window,{key:'k',ctrlKey:true});expect(document.activeElement).toBe(screen.getByRole('textbox',{name:'Busca global de canais'}));
  await user.click(screen.getByRole('link',{name:'Pular para o conteúdo'}));expect(document.activeElement?.id).toBe('main');
  await user.click(within(screen.getByRole('navigation',{name:'Navegação principal'})).getByRole('link',{name:'Sistema'}));
  await waitFor(()=>expect(screen.getByRole('heading',{level:1,name:'Sistema'})).toBeTruthy());
  expect(within(screen.getByRole('navigation')).getByRole('link',{name:'Sistema'}).getAttribute('aria-current')).toBe('page');
 });
 it('não preenche o modo real com canais de exemplo e usa apenas consultas',async()=>{
  const fetch=vi.fn(async()=>new Response('{}',{status:503}));vi.stubGlobal('fetch',fetch);render(<App initialDemo={false}/>);
  await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(9));expect(screen.getByText('DADOS DO EDGE · SOMENTE LEITURA')).toBeTruthy();expect(screen.queryByText('Globo HD')).toBeNull();expect(screen.getByText('Nenhum catálogo disponível')).toBeTruthy();
  for(const call of fetch.mock.calls as unknown as [string,RequestInit][]){expect(call[1].method).toBe('GET');}
 });
});
