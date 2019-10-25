$PROBLEM 10 mixture model and IOV on CL
$INPUT C ID DV AMT II ADDL TIME RATE HT WT CLCR SEX AGE 
EVID VIST MDV
$DATA  ./MixSim.csv IGNORE=C
$SUB ADVAN1 TRANS2
$MIX
    NSPOP=2 
    P(1)=THETA(6)
    P(2)=1.-THETA(6)     
$PK
;IOV
VST1=0
IF(VIST.EQ.1) VST1=1
VST2=0
IF(VIST.EQ.2) VST2=1
VST3=0
IF(VIST.EQ.3) VST3=1
EST=MIXEST
   IF (MIXNUM.EQ.1) THEN             
       TVCL=THETA(1)*(WT/70)**THETA(3)
   ELSE                                                 
       TVCL=THETA(5)*(WT/70)**THETA(3)
   ENDIF
                   
CL=TVCL*EXP(ETA(1) + ETA(3)*VST1 + ETA(4)*VST2 + ETA(5)*VST3)
TVV=THETA(2)*(WT/70)**THETA(4)
V=TVV*EXP(ETA(2))
S1=V
$ERROR
Y=F + F*ERR(1) + ERR(2)
IPRED=F
$THETA
 (0, 12) ;1. CL POP 1
 (0, 150);2. V
 (0.75)   ;3. WT ON CL
 (1)   ;4. WT ON V
 (0, 1) ;5. CL POP 2
 (0, 0.5, 1) ;6. PROB POP 1
$OMEGA
(0.04) ;1. CL VAR
(0.04) ;2. V VAR
;IOV
$OMEGA BLOCK(1)  0.04; interoccasion var in CL 
$OMEGA BLOCK(1) SAME
$OMEGA BLOCK(1) SAME
$SIGMA 
0.04
10
;$MSFI=./2.msf
$EST PRINT=5 MAX=9999 POSTHOC NOABORT
$COV



